package profiles

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cloak"
)

func parseOptionalBool(raw string) *bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return nil
	case "true", "1", "yes":
		v := true
		return &v
	case "false", "0", "no":
		v := false
		return &v
	default:
		return nil
	}
}

func ensureCloakProfile(name string, backend *bridge.ProfileBackend) error {
	if backend == nil || backend.Kind != "cloak" {
		return nil
	}
	normalized := normalizeProfileBackend(backend)
	if normalized == nil || normalized.Cloak == nil {
		return fmt.Errorf("cloak backend missing configuration")
	}
	if strings.TrimSpace(normalized.Cloak.ProfileID) != "" {
		backend.Cloak = normalized.Cloak
		return nil
	}

	client := cloak.NewClient(normalized.Cloak.BaseURL)
	created, err := client.CreateProfile(name, normalized.Cloak)
	if err != nil {
		return err
	}
	normalized.Cloak.ProfileID = strings.TrimSpace(created.ID)
	normalized.Cloak.BaseURL = cloak.NormalizeBaseURL(normalized.Cloak.BaseURL)
	backend.Kind = normalized.Kind
	backend.Cloak = normalized.Cloak
	backend.Steel = nil
	backend.PinchTab = nil
	return nil
}

func syncCloakProfile(name string, backend *bridge.ProfileBackend) error {
	normalized := normalizeProfileBackend(backend)
	if normalized == nil {
		return nil
	}
	if normalized.Kind != "cloak" {
		return nil
	}
	if err := ensureCloakProfile(name, normalized); err != nil {
		return err
	}
	client := cloak.NewClient(normalized.Cloak.BaseURL)
	updated, err := client.UpdateProfile(normalized.Cloak.ProfileID, name, normalized.Cloak)
	if err != nil {
		return err
	}
	if strings.TrimSpace(updated.ID) != "" {
		normalized.Cloak.ProfileID = strings.TrimSpace(updated.ID)
	}
	backend.Kind = normalized.Kind
	backend.Cloak = normalized.Cloak
	backend.Steel = nil
	backend.PinchTab = nil
	return nil
}

func deleteCloakProfile(backend *bridge.ProfileBackend) error {
	normalized := normalizeProfileBackend(backend)
	if normalized == nil || normalized.Kind != "cloak" || normalized.Cloak == nil {
		return nil
	}
	if strings.TrimSpace(normalized.Cloak.ProfileID) == "" {
		return nil
	}
	client := cloak.NewClient(normalized.Cloak.BaseURL)
	return client.DeleteProfile(normalized.Cloak.ProfileID)
}
