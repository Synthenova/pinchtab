import { useState, useEffect } from "react";
import { Input } from "../components/atoms";
import type { Profile } from "../generated/types";
import { csvToList, listToCsv } from "../pages/settings/settingsShared";

interface Props {
  profile: Profile;
  onChange: (
    name: string,
    useWhen: string,
    backend: Profile["backend"] | undefined,
  ) => void;
  minHeight?: string;
}

export default function ProfileBasicInfoPanel({
  profile,
  onChange,
  minHeight = "min-h-[180px]",
}: Props) {
  const [name, setName] = useState(profile.name);
  const [useWhen, setUseWhen] = useState(profile.useWhen || "");
  const [pinchTabProxyUrl, setPinchTabProxyUrl] = useState(
    profile.backend?.pinchtab?.proxyUrl || "",
  );
  const [pinchTabTimezone, setPinchTabTimezone] = useState(
    profile.backend?.pinchtab?.timezone || "",
  );
  const [steelProxyUrl, setSteelProxyUrl] = useState(
    profile.backend?.steel?.proxyUrl || "",
  );
  const [steelExtensionPaths, setSteelExtensionPaths] = useState<string[]>(
    profile.backend?.steel?.extensionPaths || [],
  );

  useEffect(() => {
    setName(profile.name);
    setUseWhen(profile.useWhen || "");
    setPinchTabProxyUrl(profile.backend?.pinchtab?.proxyUrl || "");
    setPinchTabTimezone(profile.backend?.pinchtab?.timezone || "");
    setSteelProxyUrl(profile.backend?.steel?.proxyUrl || "");
    setSteelExtensionPaths(profile.backend?.steel?.extensionPaths || []);
  }, [profile]);

  useEffect(() => {
    const backend =
      profile.backend?.kind === "steel"
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
    onChange(name, useWhen, backend);
  }, [
    name,
    useWhen,
    pinchTabProxyUrl,
    pinchTabTimezone,
    steelProxyUrl,
    steelExtensionPaths,
    profile.backend,
    onChange,
  ]);

  return (
    <div className="space-y-4">
      <Input
        label="Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
      />

      <div>
        <label className="dashboard-section-title mb-1 block text-[0.68rem]">
          Use this profile when
        </label>
        <textarea
          value={useWhen}
          onChange={(e) => setUseWhen(e.target.value)}
          className={`${minHeight} w-full resize-y rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary`}
        />
      </div>
      {profile.backend?.kind !== "steel" && (
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
      )}
      {profile.backend?.kind === "steel" && (
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
            onChange={(e) => setSteelExtensionPaths(csvToList(e.target.value))}
          />
        </>
      )}
    </div>
  );
}
