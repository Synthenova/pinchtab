import { useEffect, useMemo, useRef, useState, type ChangeEvent } from "react";
import { useLocation } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import { EmptyState, Button, Badge, Input, Modal } from "../components/atoms";
import * as api from "../services/api";
import type { Profile } from "../generated/types";
import type { UpdateProfileRequest } from "../services/api";
import {
  CreateProfileModal,
  StartInstanceModal,
} from "../components/molecules";
import ProfileDetailsPanel from "../profiles/ProfileDetailsPanel";

function getProfileKey(profile: Profile) {
  return profile.id || profile.name;
}

function getProfileSelectionId(profile: Profile) {
  return profile.id || profile.name;
}

function downloadJson(filename: string, value: unknown) {
  const blob = new Blob([JSON.stringify(value, null, 2)], {
    type: "application/json",
  });
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  window.URL.revokeObjectURL(url);
}

interface ProfilesLocationState {
  selectedProfileKey?: string;
}

const defaultCloudBucket = "conthunt-dev-pinchtab-profiles";
const defaultCloudPrefix = "pinchtab/profiles";
const defaultCloudCredentialPath =
  "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json";

interface CloudImportModalProps {
  open: boolean;
  onClose: () => void;
  onImported: (preferredProfileKey?: string) => Promise<void> | void;
}

function CloudImportModal({
  open,
  onClose,
  onImported,
}: CloudImportModalProps) {
  const [bucket, setBucket] = useState(defaultCloudBucket);
  const [prefix, setPrefix] = useState(defaultCloudPrefix);
  const [credentialPath, setCredentialPath] = useState(
    defaultCloudCredentialPath,
  );
  const [discovering, setDiscovering] = useState(false);
  const [importingProfileId, setImportingProfileId] = useState<string | null>(
    null,
  );
  const [discovered, setDiscovered] = useState<api.DiscoveredCloudProfile[]>(
    [],
  );
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      return;
    }
    setDiscovering(false);
    setImportingProfileId(null);
    setDiscovered([]);
    setError(null);
  }, [open]);

  const handleDiscover = async () => {
    setDiscovering(true);
    setError(null);
    try {
      const result = await api.discoverCloudProfiles({
        bucket: bucket.trim(),
        prefix: prefix.trim(),
        credentialPath: credentialPath.trim(),
      });
      setDiscovered(result.profiles);
    } catch (err) {
      console.error("Failed to discover cloud profiles", err);
      setError(err instanceof Error ? err.message : "Cloud discover failed.");
      setDiscovered([]);
    } finally {
      setDiscovering(false);
    }
  };

  const handleImport = async (profile: api.DiscoveredCloudProfile) => {
    setImportingProfileId(profile.profileId);
    setError(null);
    try {
      const result = await api.importCloudProfile({
        bucket: bucket.trim(),
        prefix: prefix.trim(),
        credentialPath: credentialPath.trim(),
        profileId: profile.profileId,
      });
      await onImported(result.name);
      onClose();
    } catch (err) {
      console.error("Failed to import cloud profile", err);
      setError(err instanceof Error ? err.message : "Cloud import failed.");
    } finally {
      setImportingProfileId(null);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="☁️ Import Cloud Profile"
      wide
      actions={
        <>
          <Button
            type="button"
            variant="secondary"
            onClick={onClose}
            disabled={discovering || !!importingProfileId}
          >
            Close
          </Button>
          <Button
            type="button"
            variant="primary"
            onClick={handleDiscover}
            loading={discovering}
            disabled={
              !bucket.trim() || !prefix.trim() || !credentialPath.trim()
            }
          >
            Scan Cloud
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Input
          label="Bucket"
          placeholder={defaultCloudBucket}
          value={bucket}
          onChange={(event) => setBucket(event.target.value)}
        />
        <Input
          label="Prefix"
          placeholder={defaultCloudPrefix}
          value={prefix}
          onChange={(event) => setPrefix(event.target.value)}
        />
        <Input
          label="Credential path"
          placeholder={defaultCloudCredentialPath}
          value={credentialPath}
          onChange={(event) => setCredentialPath(event.target.value)}
        />

        {error && (
          <div className="rounded border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}

        <div className="rounded border border-border-subtle bg-black/10">
          <div className="border-b border-border-subtle px-3 py-2 text-xs font-medium uppercase tracking-[0.18em] text-text-muted">
            Discovered Profiles
          </div>
          {discovered.length === 0 ? (
            <div className="px-3 py-4 text-sm text-text-muted">
              Scan the configured bucket to list cloud-backed profiles.
            </div>
          ) : (
            <div className="flex max-h-96 flex-col overflow-auto">
              {discovered.map((profile) => (
                <div
                  key={profile.profileId}
                  className="border-b border-border-subtle px-3 py-3 last:border-b-0"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-text-primary">
                        {profile.name || profile.profileId}
                      </div>
                      <div className="mt-1 break-all text-xs text-text-muted">
                        {profile.profileId}
                      </div>
                      <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
                        {profile.timezone && (
                          <span>TZ: {profile.timezone}</span>
                        )}
                        {profile.locale && (
                          <span>Locale: {profile.locale}</span>
                        )}
                        {profile.proxyUrl && <span>Proxy configured</span>}
                        {profile.updatedAt && (
                          <span>
                            Updated{" "}
                            {new Date(profile.updatedAt).toLocaleString()}
                          </span>
                        )}
                      </div>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="primary"
                      onClick={() => void handleImport(profile)}
                      loading={importingProfileId === profile.profileId}
                      disabled={
                        !!importingProfileId &&
                        importingProfileId !== profile.profileId
                      }
                    >
                      Attach
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}

export default function ProfilesPage() {
  const location = useLocation();
  const {
    profiles,
    instances,
    profilesLoading,
    setProfiles,
    setProfilesLoading,
    setInstances,
  } = useAppStore();
  const [showCreate, setShowCreate] = useState(false);
  const [launchProfileKey, setLaunchProfileKey] = useState<string | null>(null);
  const [selectedProfileKey, setSelectedProfileKey] = useState<string | null>(
    null,
  );
  const [selectedExportIds, setSelectedExportIds] = useState<string[]>([]);
  const [exporting, setExporting] = useState(false);
  const [importing, setImporting] = useState(false);
  const [showCloudImport, setShowCloudImport] = useState(false);
  const [banner, setBanner] = useState<string | null>(null);
  const [syncingProfileId, setSyncingProfileId] = useState<string | null>(null);
  const [finalizeProfileId, setFinalizeProfileId] = useState<string | null>(
    null,
  );
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const locationState = location.state as ProfilesLocationState | null;
  const routeSelectedProfileKey = locationState?.selectedProfileKey ?? null;

  const loadProfiles = async (preferredProfileKey?: string) => {
    setProfilesLoading(true);
    try {
      const data = await api.fetchProfiles();
      setProfiles(data);
      setSelectedExportIds((prev) =>
        prev.filter((id) =>
          data.some((profile) => getProfileSelectionId(profile) === id),
        ),
      );
      if (preferredProfileKey) {
        const preferred = data.find(
          (profile) =>
            getProfileKey(profile) === preferredProfileKey ||
            profile.name === preferredProfileKey,
        );
        if (preferred) {
          setSelectedProfileKey(getProfileKey(preferred));
        }
      }
    } catch (e) {
      console.error("Failed to load profiles", e);
    } finally {
      setProfilesLoading(false);
    }
  };

  // Load once on mount if empty — SSE handles updates
  useEffect(() => {
    if (profiles.length === 0) {
      loadProfiles(routeSelectedProfileKey ?? undefined);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!routeSelectedProfileKey || profiles.length === 0) {
      return;
    }

    const preferred = profiles.find(
      (profile) =>
        getProfileKey(profile) === routeSelectedProfileKey ||
        profile.name === routeSelectedProfileKey,
    );
    if (preferred && getProfileKey(preferred) !== selectedProfileKey) {
      setSelectedProfileKey(getProfileKey(preferred));
    }
  }, [profiles, routeSelectedProfileKey, selectedProfileKey]);

  const pollFinalizeStatus = async (profile: Profile) => {
    if (!profile.id) return;
    setFinalizeProfileId(profile.id);
    try {
      let attempts = 0;
      while (attempts < 90) {
        attempts += 1;
        const status = await api.fetchProfileFinalize(profile.id);
        await loadProfiles(profile.id);
        if (status.state === "done" || status.state === "idle") {
          setBanner(`Cloud upload completed for ${profile.name}.`);
          break;
        }
        if (status.state === "error") {
          setBanner(status.error || `Cloud upload failed for ${profile.name}.`);
          break;
        }
        await new Promise((resolve) => window.setTimeout(resolve, 1000));
      }
    } catch (e) {
      console.error("Failed to poll finalize status", e);
      setBanner(
        e instanceof Error
          ? e.message
          : `Failed to refresh cloud upload for ${profile.name}.`,
      );
    } finally {
      setFinalizeProfileId(null);
      await loadProfiles(profile.id);
    }
  };

  const handleStop = async (profile: Profile) => {
    const profileName = profile.name;
    const inst = instanceByProfile.get(profileName);
    if (!inst) return;
    try {
      await api.stopInstance(inst.id);
      const [updatedInstances] = await Promise.all([
        api.fetchInstances(),
        loadProfiles(profile.id),
      ]);
      setInstances(updatedInstances);
      const refreshed = await api.fetchProfiles();
      const stoppedProfile =
        refreshed.find((item) => item.id === profile.id) || profile;
      setProfiles(refreshed);
      if (stoppedProfile.cloudStatus?.state === "stopped-uploading") {
        setBanner(`Stopping ${profile.name}. Cloud upload is continuing.`);
        await pollFinalizeStatus(stoppedProfile);
      }
    } catch (e) {
      console.error("Failed to stop instance", e);
    }
  };

  const handleDelete = async () => {
    if (!selectedProfile?.id) return;
    try {
      await api.deleteProfile(selectedProfile.id);
      setSelectedProfileKey(null);
      loadProfiles();
    } catch (e) {
      console.error("Failed to delete profile", e);
    }
  };

  const handleSave = async (values: UpdateProfileRequest) => {
    if (!selectedProfile?.id) return;
    try {
      const updated = await api.updateProfile(selectedProfile.id, {
        name:
          values.name !== undefined && values.name !== selectedProfile.name
            ? values.name
            : undefined,
        useWhen:
          values.useWhen !== undefined &&
          values.useWhen !== selectedProfile.useWhen
            ? values.useWhen
            : undefined,
        backend:
          JSON.stringify(values.backend || null) !==
          JSON.stringify(selectedProfile.backend || null)
            ? values.backend
            : undefined,
      });
      loadProfiles(updated.id || selectedProfile.id);
    } catch (e) {
      console.error("Failed to update profile", e);
    }
  };

  const handleSync = async (profile: Profile) => {
    if (!profile.id || syncingProfileId) return;
    setSyncingProfileId(profile.id);
    setBanner(null);
    try {
      const status = await api.startProfileSync(profile.id);
      setBanner(
        status.state === "ready"
          ? `Profile ${profile.name} is already synced.`
          : `Started cloud sync for ${profile.name}.`,
      );

      let attempts = 0;
      while (attempts < 90) {
        attempts += 1;
        const next = await api.fetchProfileSync(profile.id);
        await loadProfiles(profile.id);
        if (next.state === "ready") {
          setBanner(`Cloud sync completed for ${profile.name}.`);
          break;
        }
        if (next.state === "error") {
          setBanner(next.error || `Cloud sync failed for ${profile.name}.`);
          break;
        }
        await new Promise((resolve) => window.setTimeout(resolve, 1000));
      }
    } catch (e) {
      console.error("Failed to sync profile", e);
      setBanner(
        e instanceof Error ? e.message : `Failed to sync ${profile.name}.`,
      );
    } finally {
      setSyncingProfileId(null);
      await loadProfiles(profile.id);
    }
  };

  const handleRetryUpload = async (profile: Profile) => {
    if (!profile.id || finalizeProfileId) return;
    setFinalizeProfileId(profile.id);
    setBanner(null);
    try {
      await api.retryProfileFinalize(profile.id);
      setBanner(`Retrying cloud upload for ${profile.name}.`);
      await pollFinalizeStatus(profile);
    } catch (e) {
      console.error("Failed to retry upload", e);
      setBanner(
        e instanceof Error
          ? e.message
          : `Failed to retry upload for ${profile.name}.`,
      );
      setFinalizeProfileId(null);
    }
  };

  const handleDiscardChanges = async (profile: Profile) => {
    if (!profile.id || finalizeProfileId) return;
    setFinalizeProfileId(profile.id);
    setBanner(null);
    try {
      await api.discardProfileFinalize(profile.id);
      await loadProfiles(profile.id);
      setBanner(`Discarded unsynced local changes for ${profile.name}.`);
    } catch (e) {
      console.error("Failed to discard local cloud changes", e);
      setBanner(
        e instanceof Error
          ? e.message
          : `Failed to discard local changes for ${profile.name}.`,
      );
    } finally {
      setFinalizeProfileId(null);
    }
  };

  const toggleExportSelection = (profile: Profile) => {
    const selectionId = getProfileSelectionId(profile);
    setSelectedExportIds((prev) =>
      prev.includes(selectionId)
        ? prev.filter((id) => id !== selectionId)
        : [...prev, selectionId],
    );
  };

  const handleExport = async (mode: "selected" | "all") => {
    if (mode === "selected" && selectedExportIds.length === 0) {
      setBanner("Select at least one profile to export.");
      return;
    }
    setExporting(true);
    setBanner(null);
    try {
      const bundle = await api.exportProfileConfigs(
        mode === "selected" ? { ids: selectedExportIds } : {},
      );
      const label =
        mode === "selected" && bundle.profiles.length === 1
          ? bundle.profiles[0].name
          : `${bundle.profiles.length}-profiles`;
      downloadJson(`pinchtab-${label}.json`, bundle);
      setBanner(
        `Exported ${bundle.profiles.length} profile${bundle.profiles.length === 1 ? "" : "s"}.`,
      );
    } catch (e) {
      console.error("Failed to export profiles", e);
      setBanner("Export failed.");
    } finally {
      setExporting(false);
    }
  };

  const handleImportClick = () => {
    fileInputRef.current?.click();
  };

  const handleImportFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const [file] = Array.from(event.target.files || []);
    event.target.value = "";
    if (!file) return;

    setImporting(true);
    setBanner(null);
    try {
      const text = await file.text();
      const payload = JSON.parse(text) as api.ProfileConfigBundle;
      const imported = await api.importProfileConfigs({
        bundle: payload,
      });
      await loadProfiles();
      setBanner(
        `Imported ${imported.count} profile${imported.count === 1 ? "" : "s"}.`,
      );
    } catch (e) {
      console.error("Failed to import profiles", e);
      setBanner("Import failed. Check that the JSON bundle is valid.");
    } finally {
      setImporting(false);
    }
  };

  const instanceByProfile = useMemo(
    () => new Map(instances.map((i) => [i.profileName, i])),
    [instances],
  );
  const orderedProfiles = useMemo(() => {
    const running: Profile[] = [];
    const stopped: Profile[] = [];

    profiles.forEach((profile) => {
      if (instanceByProfile.get(profile.name)?.status === "running") {
        running.push(profile);
        return;
      }
      stopped.push(profile);
    });

    return [...running, ...stopped];
  }, [instanceByProfile, profiles]);
  const runningProfileKeys = orderedProfiles
    .filter(
      (profile) => instanceByProfile.get(profile.name)?.status === "running",
    )
    .map((profile) => getProfileKey(profile));
  const singleRunningProfileKey =
    runningProfileKeys.length === 1 ? runningProfileKeys[0] : null;
  const selectedProfile =
    orderedProfiles.find(
      (profile) => getProfileKey(profile) === selectedProfileKey,
    ) || null;
  const launchProfile =
    orderedProfiles.find(
      (profile) => getProfileKey(profile) === launchProfileKey,
    ) || null;
  useEffect(() => {
    if (orderedProfiles.length === 0) {
      setSelectedProfileKey(null);
      return;
    }

    const hasValidSelection =
      !!selectedProfileKey &&
      orderedProfiles.some(
        (profile) => getProfileKey(profile) === selectedProfileKey,
      );

    if (!hasValidSelection) {
      setSelectedProfileKey(
        singleRunningProfileKey ?? getProfileKey(orderedProfiles[0]),
      );
    }
  }, [orderedProfiles, selectedProfileKey, singleRunningProfileKey]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="h-full">
          {profilesLoading && profiles.length === 0 ? (
            <div className="flex items-center justify-center py-16 text-text-muted">
              Loading profiles...
            </div>
          ) : profiles.length === 0 ? (
            <EmptyState
              title="No profiles yet"
              description="Click New Profile to create one"
              action={
                <Button variant="primary" onClick={() => setShowCreate(true)}>
                  New Profile
                </Button>
              }
            />
          ) : (
            <div className="dashboard-panel flex h-full min-h-0 flex-col overflow-hidden rounded-none! border-t-0 lg:flex-row">
              <div className="flex max-h-88 w-full shrink-0 flex-col overflow-hidden border-r border-border-subtle bg-bg-surface/50 lg:max-h-none lg:w-80">
                <div className="border-b border-border-subtle px-4 py-2.5">
                  <span className="text-xs font-medium text-text-muted">
                    Profiles
                  </span>
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={handleImportClick}
                      loading={importing}
                    >
                      Import
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setShowCloudImport(true)}
                    >
                      Import Cloud
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      onClick={() => handleExport("selected")}
                      disabled={selectedExportIds.length === 0}
                      loading={exporting}
                    >
                      Export Selected
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="primary"
                      className="ml-auto"
                      onClick={() => setShowCreate(true)}
                    >
                      New Profile
                    </Button>
                  </div>
                </div>

                <div className="border-b border-border-subtle px-4 py-2.5">
                  <div className="flex items-center justify-between gap-3">
                    <label className="flex items-center gap-2 text-xs text-text-muted">
                      <input
                        type="checkbox"
                        className="h-3.5 w-3.5 rounded border-border-subtle bg-transparent"
                        checked={
                          orderedProfiles.length > 0 &&
                          selectedExportIds.length === orderedProfiles.length
                        }
                        onChange={(event) => {
                          setSelectedExportIds(
                            event.target.checked
                              ? orderedProfiles.map((profile) =>
                                  getProfileSelectionId(profile),
                                )
                              : [],
                          );
                        }}
                      />
                      Select all
                    </label>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => handleExport("all")}
                      loading={exporting}
                    >
                      Export All
                    </Button>
                  </div>
                  {banner && (
                    <div className="mt-2 text-xs text-text-muted">{banner}</div>
                  )}
                </div>

                <div className="flex-1 overflow-auto">
                  <div>
                    {orderedProfiles.map((profile) => {
                      const instance = instanceByProfile.get(profile.name);
                      const isSelected =
                        getProfileKey(profile) === selectedProfileKey;
                      const isMarkedForExport = selectedExportIds.includes(
                        getProfileSelectionId(profile),
                      );
                      const accountText =
                        profile.accountEmail ||
                        profile.accountName ||
                        "No account";
                      const statusVariant =
                        instance?.status === "running"
                          ? "success"
                          : instance?.status === "error"
                            ? "danger"
                            : "default";
                      const statusLabel =
                        instance?.status === "running"
                          ? `:${instance.port}`
                          : instance?.status === "error"
                            ? "error"
                            : "stopped";
                      const cloudLabel =
                        profile.cloudStatus?.state === "in-use"
                          ? `in use · ${profile.cloudStatus.leaseMachine || "remote"}`
                          : [
                                "queued",
                                "checking",
                                "downloading",
                                "extracting",
                              ].includes(profile.cloudStatus?.state || "")
                            ? `syncing · ${profile.cloudStatus?.state}`
                            : profile.cloudStatus?.state === "sync-required"
                              ? "sync required"
                              : profile.cloudStatus?.state === "error"
                                ? "cloud error"
                                : profile.backend?.pinchtab?.cloud?.enabled
                                  ? "cloud"
                                  : "";

                      return (
                        <div
                          key={getProfileKey(profile)}
                          className={`w-full border-b border-border-subtle px-3 py-2.5 text-left transition-colors ${
                            isSelected
                              ? "bg-bg-hover text-text-primary"
                              : "hover:bg-bg-hover/50"
                          }`}
                        >
                          <div className="flex items-start gap-3">
                            <label className="mt-0.5 flex shrink-0 items-center">
                              <input
                                type="checkbox"
                                aria-label={`Select ${profile.name} for export`}
                                className="h-3.5 w-3.5 rounded border-border-subtle bg-transparent"
                                checked={isMarkedForExport}
                                onChange={() => toggleExportSelection(profile)}
                                onClick={(event) => event.stopPropagation()}
                              />
                            </label>
                            <button
                              type="button"
                              onClick={() =>
                                setSelectedProfileKey(getProfileKey(profile))
                              }
                              className="min-w-0 flex-1 text-left"
                            >
                              <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0">
                                  <div className="truncate text-sm font-semibold text-text-primary">
                                    {profile.name}
                                  </div>
                                  <div className="mt-1 text-xs text-text-muted">
                                    {accountText}
                                  </div>
                                  {cloudLabel && (
                                    <div className="mt-1 text-[11px] text-text-muted">
                                      {cloudLabel}
                                    </div>
                                  )}
                                </div>
                                <Badge variant={statusVariant}>
                                  {statusLabel}
                                </Badge>
                              </div>

                              {profile.useWhen && (
                                <div className="mt-3 line-clamp-2 text-xs leading-5 text-text-secondary">
                                  {profile.useWhen}
                                </div>
                              )}
                            </button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>

              <div className="min-h-0 min-w-0 flex-1">
                <ProfileDetailsPanel
                  profile={selectedProfile}
                  instance={
                    selectedProfile
                      ? instanceByProfile.get(selectedProfile.name)
                      : undefined
                  }
                  onLaunch={() =>
                    selectedProfile &&
                    setLaunchProfileKey(getProfileKey(selectedProfile))
                  }
                  onStop={() => selectedProfile && handleStop(selectedProfile)}
                  onSync={() =>
                    selectedProfile && void handleSync(selectedProfile)
                  }
                  onRetryUpload={() =>
                    selectedProfile && void handleRetryUpload(selectedProfile)
                  }
                  onDiscardChanges={() =>
                    selectedProfile &&
                    void handleDiscardChanges(selectedProfile)
                  }
                  onSave={handleSave}
                  onDelete={handleDelete}
                  syncLoading={syncingProfileId === selectedProfile?.id}
                  finalizeLoading={finalizeProfileId === selectedProfile?.id}
                />
              </div>
            </div>
          )}
        </div>
      </div>

      <CreateProfileModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onCreated={loadProfiles}
      />

      <CloudImportModal
        open={showCloudImport}
        onClose={() => setShowCloudImport(false)}
        onImported={loadProfiles}
      />

      <StartInstanceModal
        open={!!launchProfile}
        profile={launchProfile}
        onClose={() => setLaunchProfileKey(null)}
      />

      <input
        ref={fileInputRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        onChange={handleImportFile}
      />
    </div>
  );
}
