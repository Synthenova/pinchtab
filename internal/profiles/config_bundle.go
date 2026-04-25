package profiles

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

const profileConfigBundleVersion = "pinchtab.profile-config.v1"

type ProfileConfigRecord struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	UseWhen     string                 `json:"useWhen,omitempty"`
	Backend     *bridge.ProfileBackend `json:"backend,omitempty"`
}

type ProfileConfigBundle struct {
	Version    string                `json:"version"`
	ExportedAt time.Time             `json:"exportedAt"`
	Profiles   []ProfileConfigRecord `json:"profiles"`
}

type ProfileConfigImportOptions struct {
	Overwrite               bool `json:"overwrite,omitempty"`
	PreserveCloakProfileIDs bool `json:"preserveCloakProfileIds,omitempty"`
}

func cloneProfileBackend(backend *bridge.ProfileBackend) *bridge.ProfileBackend {
	if backend == nil {
		return nil
	}
	data, err := json.Marshal(backend)
	if err != nil {
		return nil
	}
	var cloned bridge.ProfileBackend
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return normalizeProfileBackend(&cloned)
}

func exportRecordForMeta(name string, meta ProfileMeta) ProfileConfigRecord {
	record := ProfileConfigRecord{
		ID:          meta.ID,
		Name:        name,
		Description: meta.Description,
		UseWhen:     meta.UseWhen,
		Backend:     cloneProfileBackend(meta.Backend),
	}
	if record.ID == "" {
		record.ID = profileID(name)
	}
	return record
}

func (pm *ProfileManager) ExportConfigBundle(ids, names []string) (*ProfileConfigBundle, error) {
	profiles, err := pm.List()
	if err != nil {
		return nil, err
	}

	selectedByName := make(map[string]bridge.ProfileInfo, len(profiles))
	selectedByID := make(map[string]bridge.ProfileInfo, len(profiles))
	for _, profile := range profiles {
		selectedByName[profile.Name] = profile
		selectedByID[profile.ID] = profile
	}

	var chosen []bridge.ProfileInfo
	if len(ids) == 0 && len(names) == 0 {
		for _, profile := range profiles {
			if profile.Temporary {
				continue
			}
			chosen = append(chosen, profile)
		}
	} else {
		seen := map[string]struct{}{}
		for _, id := range normalizeStringList(ids) {
			profile, ok := selectedByID[id]
			if !ok {
				return nil, fmt.Errorf("profile %q not found", id)
			}
			if _, dup := seen[profile.Name]; dup {
				continue
			}
			seen[profile.Name] = struct{}{}
			chosen = append(chosen, profile)
		}
		for _, name := range normalizeStringList(names) {
			profile, ok := selectedByName[name]
			if !ok {
				return nil, fmt.Errorf("profile %q not found", name)
			}
			if _, dup := seen[profile.Name]; dup {
				continue
			}
			seen[profile.Name] = struct{}{}
			chosen = append(chosen, profile)
		}
	}

	slices.SortFunc(chosen, func(a, b bridge.ProfileInfo) int {
		return strings.Compare(a.Name, b.Name)
	})

	records := make([]ProfileConfigRecord, 0, len(chosen))
	for _, profile := range chosen {
		meta, err := pm.Meta(profile.Name)
		if err != nil {
			return nil, err
		}
		records = append(records, exportRecordForMeta(profile.Name, meta))
	}

	return &ProfileConfigBundle{
		Version:    profileConfigBundleVersion,
		ExportedAt: time.Now().UTC(),
		Profiles:   records,
	}, nil
}

func importUpdatesForRecord(record ProfileConfigRecord, preserveCloakProfileIDs bool) map[string]string {
	updates := map[string]string{
		"description": record.Description,
		"useWhen":     record.UseWhen,
	}
	backend := cloneProfileBackend(record.Backend)
	if backend != nil && backend.Kind == "cloak" && backend.Cloak != nil && !preserveCloakProfileIDs {
		backend.Cloak.ProfileID = ""
	}
	appendBackendUpdateFields(updates, backend, preserveCloakProfileIDs)
	return updates
}

func appendBackendUpdateFields(updates map[string]string, backend *bridge.ProfileBackend, preserveCloakProfileIDs bool) {
	if backend == nil {
		return
	}
	updates["backend.kind"] = backend.Kind
	if backend.Steel != nil {
		updates["backend.steel.proxyUrl"] = backend.Steel.ProxyURL
		updates["backend.steel.extensionPaths"] = encodeProfileStringList(backend.Steel.ExtensionPaths)
	}
	if backend.PinchTab != nil {
		updates["backend.pinchtab.proxyUrl"] = backend.PinchTab.ProxyURL
		updates["backend.pinchtab.timezone"] = backend.PinchTab.Timezone
		updates["backend.pinchtab.locale"] = backend.PinchTab.Locale
		updates["backend.pinchtab.binary"] = backend.PinchTab.Binary
		updates["backend.pinchtab.browserVersion"] = backend.PinchTab.BrowserVersion
		updates["backend.pinchtab.launchArgs"] = encodeProfileStringList(backend.PinchTab.LaunchArgs)
	}
	if backend.Cloak != nil {
		updates["backend.cloak.baseUrl"] = backend.Cloak.BaseURL
		if preserveCloakProfileIDs && strings.TrimSpace(backend.Cloak.ProfileID) != "" {
			updates["backend.cloak.profileId"] = backend.Cloak.ProfileID
		}
		updates["backend.cloak.proxyUrl"] = backend.Cloak.ProxyURL
		updates["backend.cloak.timezone"] = backend.Cloak.Timezone
		updates["backend.cloak.locale"] = backend.Cloak.Locale
		updates["backend.cloak.platform"] = backend.Cloak.Platform
		updates["backend.cloak.userAgent"] = backend.Cloak.UserAgent
		updates["backend.cloak.launchArgs"] = encodeProfileStringList(backend.Cloak.LaunchArgs)
		updates["backend.cloak.notes"] = backend.Cloak.Notes
		updates["backend.cloak.headless"] = formatOptionalBool(backend.Cloak.Headless)
		updates["backend.cloak.humanize"] = formatOptionalBool(backend.Cloak.Humanize)
		updates["backend.cloak.geoip"] = formatOptionalBool(backend.Cloak.GeoIP)
	}
}

func (pm *ProfileManager) ImportConfigBundle(bundle ProfileConfigBundle, options ProfileConfigImportOptions) ([]string, error) {
	if strings.TrimSpace(bundle.Version) != "" && bundle.Version != profileConfigBundleVersion {
		return nil, fmt.Errorf("unsupported profile bundle version %q", bundle.Version)
	}
	if len(bundle.Profiles) == 0 {
		return nil, fmt.Errorf("profiles required")
	}

	seen := map[string]struct{}{}
	for _, record := range bundle.Profiles {
		name := strings.TrimSpace(record.Name)
		if name == "" {
			return nil, fmt.Errorf("profile name required")
		}
		if err := ValidateProfileName(name); err != nil {
			return nil, err
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("duplicate profile %q in import bundle", name)
		}
		seen[name] = struct{}{}
		if pm.Exists(name) && !options.Overwrite {
			return nil, fmt.Errorf("profile %q already exists", name)
		}
	}

	imported := make([]string, 0, len(bundle.Profiles))
	for _, record := range bundle.Profiles {
		name := strings.TrimSpace(record.Name)
		backend := cloneProfileBackend(record.Backend)
		if backend != nil && backend.Kind == "cloak" && backend.Cloak != nil && !options.PreserveCloakProfileIDs {
			backend.Cloak.ProfileID = ""
		}
		if !pm.Exists(name) {
			if err := pm.CreateWithMeta(name, ProfileMeta{
				Name:        name,
				Description: record.Description,
				UseWhen:     record.UseWhen,
				Backend:     backend,
			}); err != nil {
				return imported, err
			}
			imported = append(imported, name)
			continue
		}
		if err := pm.UpdateMeta(name, importUpdatesForRecord(record, options.PreserveCloakProfileIDs)); err != nil {
			return imported, err
		}
		imported = append(imported, name)
	}

	return imported, nil
}
