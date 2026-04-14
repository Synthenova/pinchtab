import { useEffect, useId, useState } from "react";
import { Button, Input, Modal } from "../atoms";
import * as api from "../../services/api";
import { csvToList, listToCsv } from "../../pages/settings/settingsShared";

interface Props {
  open: boolean;
  onClose: () => void;
  onCreated: (preferredProfileKey: string) => void | Promise<void>;
}

export default function CreateProfileModal({
  open,
  onClose,
  onCreated,
}: Props) {
  const [createName, setCreateName] = useState("");
  const [createUseWhen, setCreateUseWhen] = useState("");
  const [createSource, setCreateSource] = useState("");
  const [backendKind, setBackendKind] = useState<"pinchtab" | "steel">(
    "pinchtab",
  );
  const [pinchTabProxyUrl, setPinchTabProxyUrl] = useState("");
  const [pinchTabTimezone, setPinchTabTimezone] = useState("");
  const [steelProxyUrl, setSteelProxyUrl] = useState("");
  const [steelExtensionPaths, setSteelExtensionPaths] = useState<string[]>([]);
  const [createLoading, setCreateLoading] = useState(false);
  const backendSelectId = useId();

  useEffect(() => {
    if (open) {
      return;
    }

    setCreateName("");
    setCreateUseWhen("");
    setCreateSource("");
    setBackendKind("pinchtab");
    setPinchTabProxyUrl("");
    setPinchTabTimezone("");
    setSteelProxyUrl("");
    setSteelExtensionPaths([]);
    setCreateLoading(false);
  }, [open]);

  const handleCreate = async () => {
    if (!createName.trim() || createLoading) return;

    const backend =
      backendKind === "steel"
        ? {
            kind: "steel",
            steel: {
              proxyUrl: steelProxyUrl.trim() || undefined,
              extensionPaths:
                steelExtensionPaths.length > 0
                  ? steelExtensionPaths
                  : undefined,
            },
          }
        : {
            kind: "pinchtab",
            ...(pinchTabProxyUrl.trim() || pinchTabTimezone.trim()
              ? {
                  pinchtab: {
                    proxyUrl: pinchTabProxyUrl.trim() || undefined,
                    timezone: pinchTabTimezone.trim() || undefined,
                  },
                }
              : {}),
          };

    setCreateLoading(true);
    try {
      const created = await api.createProfile({
        name: createName.trim(),
        useWhen: createUseWhen.trim() || undefined,
        backend,
      });
      onClose();
      await onCreated(created.id || created.name);
    } catch (e) {
      console.error("Failed to create profile", e);
    } finally {
      setCreateLoading(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="📁 New Profile"
      wide
      actions={
        <>
          <Button
            variant="secondary"
            disabled={createLoading}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleCreate}
            disabled={!createName.trim()}
            loading={createLoading}
          >
            Create
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Input
          label="Name"
          placeholder="e.g. personal, work, scraping"
          value={createName}
          onChange={(e) => setCreateName(e.target.value)}
        />
        <Input
          label="Use this profile when (helps agents pick the right profile)"
          placeholder="e.g. I need to access Gmail for the team account"
          value={createUseWhen}
          onChange={(e) => setCreateUseWhen(e.target.value)}
        />
        <Input
          label="Import from (optional — Chrome user data path)"
          placeholder="e.g. /Users/you/Library/Application Support/Google/Chrome"
          value={createSource}
          onChange={(e) => setCreateSource(e.target.value)}
        />
        <div className="flex flex-col gap-1.5">
          <label
            htmlFor={backendSelectId}
            className="dashboard-section-title text-[0.68rem]"
          >
            Browser backend
          </label>
          <select
            id={backendSelectId}
            value={backendKind}
            onChange={(e) =>
              setBackendKind(e.target.value as "pinchtab" | "steel")
            }
            className="rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
          >
            <option value="pinchtab">PinchTab (default)</option>
            <option value="steel">Steel Browser</option>
          </select>
          <span className="text-xs text-text-muted">
            PinchTab will use the selected backend settings when the profile is
            started.
          </span>
        </div>
        {backendKind === "pinchtab" ? (
          <>
            <Input
              label="Proxy / IP (optional)"
              placeholder="http://user:pass@host:port"
              value={pinchTabProxyUrl}
              onChange={(e) => setPinchTabProxyUrl(e.target.value)}
            />
            <Input
              label="Timezone (optional)"
              placeholder="Asia/Kolkata"
              value={pinchTabTimezone}
              onChange={(e) => setPinchTabTimezone(e.target.value)}
            />
          </>
        ) : (
          <>
            <Input
              label="Proxy / IP (optional)"
              placeholder="http://user:pass@host:port"
              value={steelProxyUrl}
              onChange={(e) => setSteelProxyUrl(e.target.value)}
            />
            <Input
              label="Extensions (optional — unpacked extension directories, comma-separated)"
              placeholder="/path/to/ext-one, /path/to/ext-two"
              value={listToCsv(steelExtensionPaths)}
              onChange={(e) =>
                setSteelExtensionPaths(csvToList(e.target.value))
              }
            />
          </>
        )}
      </div>
    </Modal>
  );
}
