import { useCallback, useEffect, useMemo, useState } from "react";
import { Button, Input, Modal } from "../atoms";
import { useAppStore } from "../../stores/useAppStore";
import * as api from "../../services/api";
import type { LaunchInstanceRequest, Profile } from "../../generated/types";

interface Props {
  open: boolean;
  profile: Profile | null;
  onClose: () => void;
}

export default function StartInstanceModal({ open, profile, onClose }: Props) {
  const { setInstances } = useAppStore();
  const [port, setPort] = useState("");
  const [headless, setHeadless] = useState(false);
  const [launchError, setLaunchError] = useState("");
  const [launchLoading, setLaunchLoading] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState("");
  const [syncStatus, setSyncStatus] = useState<api.ProfileSyncStatus | null>(
    null,
  );
  const [pendingLaunch, setPendingLaunch] = useState(false);

  useEffect(() => {
    if (open) {
      setLaunchError("");
      setCopyFeedback("");
      setSyncStatus(null);
      setPendingLaunch(false);
      return;
    }

    setPort("");
    setHeadless(false);
    setLaunchError("");
    setLaunchLoading(false);
    setCopyFeedback("");
    setSyncStatus(null);
    setPendingLaunch(false);
  }, [open, profile?.id, profile?.name]);

  const launchCommand = useMemo(() => {
    if (!profile) return "";

    const payload: LaunchInstanceRequest = {
      profileId: profile.id || profile.name,
      mode: headless ? undefined : "headed",
      port: port.trim() || undefined,
    };

    return `curl -X POST http://localhost:9867/instances/start -H "Content-Type: application/json" -d '${JSON.stringify(payload)}'`;
  }, [headless, port, profile]);

  const handleLaunch = useCallback(
    async (retry = false) => {
      if (!profile || launchLoading) return;

      if (!retry) {
        setLaunchError("");
        setSyncStatus(null);
      }
      setLaunchLoading(true);

      try {
        const payload: LaunchInstanceRequest = {
          profileId: profile.id || profile.name,
          port: port.trim() || undefined,
          mode: headless ? undefined : "headed",
        };

        await api.launchInstance(payload);
        const updated = await api.fetchInstances();
        setInstances(updated);
        setPendingLaunch(false);
        onClose();
      } catch (e) {
        console.error("Launch failed:", e);
        if (api.isApiError(e) && e.code === "profile_sync_in_progress") {
          const sync = (e.details?.sync ||
            null) as api.ProfileSyncStatus | null;
          setSyncStatus(sync);
          setPendingLaunch(true);
          setLaunchError("Syncing profile before launch...");
          return;
        }
        const msg =
          e instanceof Error ? e.message : "Failed to launch instance";
        setPendingLaunch(false);
        setLaunchError(msg);
      } finally {
        setLaunchLoading(false);
      }
    },
    [headless, launchLoading, onClose, port, profile, setInstances],
  );

  useEffect(() => {
    if (!open || !profile || !pendingLaunch) {
      return;
    }

    let cancelled = false;
    let timer: number | undefined;

    const poll = async () => {
      try {
        const status = await api.fetchProfileSync(profile.id || profile.name);
        if (cancelled) return;
        setSyncStatus(status);
        if (status.state === "ready") {
          setPendingLaunch(false);
          void handleLaunch(true);
          return;
        }
        if (status.state === "error") {
          setPendingLaunch(false);
          setLaunchError(status.error || "Profile sync failed");
          return;
        }
      } catch (error) {
        if (cancelled) return;
        setPendingLaunch(false);
        setLaunchError(
          error instanceof Error ? error.message : "Failed to poll sync status",
        );
        return;
      }
      timer = window.setTimeout(poll, 1000);
    };

    void poll();
    return () => {
      cancelled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [handleLaunch, open, pendingLaunch, profile]);

  const handleCopyCommand = async () => {
    try {
      await navigator.clipboard.writeText(launchCommand);
      setCopyFeedback("Copied!");
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback("Failed to copy");
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="🖥️ Start Profile"
      actions={
        <>
          <Button
            variant="secondary"
            disabled={launchLoading || pendingLaunch}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={() => void handleLaunch()}
            loading={launchLoading || pendingLaunch}
          >
            {pendingLaunch ? "Syncing…" : "Start"}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {launchError && (
          <div className="rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {launchError}
          </div>
        )}
        {syncStatus &&
          ["queued", "checking", "downloading", "extracting", "ready"].includes(
            syncStatus.state || "",
          ) && (
            <div className="rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-secondary">
              <div className="font-medium text-text-primary">
                Cloud sync: {syncStatus.state}
              </div>
              {typeof syncStatus.progress === "number" &&
                syncStatus.progress > 0 && (
                  <div className="mt-2 h-2 overflow-hidden rounded bg-bg-hover">
                    <div
                      className="h-full bg-accent-primary transition-all"
                      style={{
                        width: `${Math.max(4, Math.min(100, syncStatus.progress))}%`,
                      }}
                    />
                  </div>
                )}
              <div className="mt-1 text-xs text-text-muted">
                {typeof syncStatus.progress === "number"
                  ? `${syncStatus.progress}%`
                  : "Preparing local cloud cache"}
                {syncStatus.bytesTotal
                  ? ` · ${syncStatus.bytesDone || 0} / ${syncStatus.bytesTotal} bytes`
                  : ""}
              </div>
            </div>
          )}
        <Input
          label="Port"
          placeholder="Auto-select from configured range"
          value={port}
          onChange={(e) => setPort(e.target.value)}
        />
        <p className="-mt-2 text-xs text-text-muted">
          Leave blank to auto-select a free port from the configured instance
          port range.
        </p>
        <label className="flex items-center gap-2 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={headless}
            onChange={(e) => setHeadless(e.target.checked)}
            className="h-4 w-4"
          />
          Headless (best for Docker/VPS)
        </label>

        <div>
          <label className="mb-1 block text-xs text-text-muted">
            Direct launch command (backup)
          </label>
          <textarea
            readOnly
            value={launchCommand}
            className="h-20 w-full resize-none rounded border border-border-subtle bg-bg-elevated px-3 py-2 font-mono text-xs text-text-secondary"
          />
          <div className="mt-2 flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={handleCopyCommand}>
              Copy Command
            </Button>
            {copyFeedback && (
              <span className="text-xs text-success">{copyFeedback}</span>
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
}
