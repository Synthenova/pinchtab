import { useState } from "react";
import { Button } from "../components/atoms";
import type { Profile, Instance } from "../generated/types";

interface Props {
  profile: Profile;
  instance?: Instance;
  onLaunch: () => void;
  onStop: () => void;
  onSync?: () => void;
  onSave: () => void;
  onDelete: () => void;
  isSaveDisabled: boolean;
  isLaunchDisabled?: boolean;
  launchDisabledReason?: string;
  syncLoading?: boolean;
}

export default function ProfileToolbarButtons({
  profile,
  instance,
  onLaunch,
  onStop,
  onSync,
  onSave,
  onDelete,
  isSaveDisabled,
  isLaunchDisabled = false,
  launchDisabledReason,
  syncLoading = false,
}: Props) {
  const [copyFeedback, setCopyFeedback] = useState("");
  const isRunning = instance?.status === "running";
  const cloudEnabled = !!profile.backend?.pinchtab?.cloud?.enabled;

  const handleCopyId = async () => {
    if (!profile.id) return;
    try {
      await navigator.clipboard.writeText(profile.id);
      setCopyFeedback("Copied");
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback("Failed");
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      {profile.id && (
        <Button size="sm" variant="secondary" onClick={handleCopyId}>
          {copyFeedback || "Copy ID"}
        </Button>
      )}
      <Button size="sm" variant="secondary" onClick={onDelete}>
        Delete
      </Button>
      {cloudEnabled && onSync && (
        <Button
          size="sm"
          variant="secondary"
          onClick={onSync}
          loading={syncLoading}
          disabled={isRunning}
          title={isRunning ? "Stop the browser before syncing" : undefined}
        >
          Sync
        </Button>
      )}
      <Button
        size="sm"
        variant="primary"
        onClick={onSave}
        disabled={isSaveDisabled}
      >
        Save
      </Button>
      {isRunning ? (
        <Button size="sm" variant="danger" onClick={onStop}>
          Stop
        </Button>
      ) : (
        <Button
          size="sm"
          variant="primary"
          onClick={onLaunch}
          disabled={isLaunchDisabled}
          title={launchDisabledReason}
        >
          Start
        </Button>
      )}
    </div>
  );
}
