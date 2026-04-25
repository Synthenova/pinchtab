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
  const [pinchTabCloudEnabled, setPinchTabCloudEnabled] = useState(
    profile.backend?.pinchtab?.cloud?.enabled ?? false,
  );
  const [pinchTabCloudBucket, setPinchTabCloudBucket] = useState(
    profile.backend?.pinchtab?.cloud?.bucket ||
      "conthunt-dev-pinchtab-profiles",
  );
  const [pinchTabCloudPrefix, setPinchTabCloudPrefix] = useState(
    profile.backend?.pinchtab?.cloud?.prefix || "pinchtab/profiles",
  );
  const [pinchTabCloudCredentialPath, setPinchTabCloudCredentialPath] =
    useState(profile.backend?.pinchtab?.cloud?.credentialPath || "");
  const [pinchTabKeepLocalCache, setPinchTabKeepLocalCache] = useState(
    profile.backend?.pinchtab?.cloud?.keepLocalCache ?? true,
  );
  const [steelProxyUrl, setSteelProxyUrl] = useState(
    profile.backend?.steel?.proxyUrl || "",
  );
  const [steelExtensionPaths, setSteelExtensionPaths] = useState<string[]>(
    profile.backend?.steel?.extensionPaths || [],
  );
  const [cloakBaseUrl, setCloakBaseUrl] = useState(
    profile.backend?.cloak?.baseUrl || "http://127.0.0.1:8080",
  );
  const [cloakProxyUrl, setCloakProxyUrl] = useState(
    profile.backend?.cloak?.proxyUrl || "",
  );
  const [cloakTimezone, setCloakTimezone] = useState(
    profile.backend?.cloak?.timezone || "",
  );
  const [cloakLocale, setCloakLocale] = useState(
    profile.backend?.cloak?.locale || "",
  );
  const [cloakLaunchArgs, setCloakLaunchArgs] = useState<string[]>(
    profile.backend?.cloak?.launchArgs || [],
  );
  const [cloakHeadless, setCloakHeadless] = useState(
    profile.backend?.cloak?.headless ?? true,
  );
  const [cloakHumanize, setCloakHumanize] = useState(
    profile.backend?.cloak?.humanize ?? true,
  );
  const [cloakGeoip, setCloakGeoip] = useState(
    profile.backend?.cloak?.geoip ?? false,
  );

  useEffect(() => {
    setName(profile.name);
    setUseWhen(profile.useWhen || "");
    setPinchTabProxyUrl(profile.backend?.pinchtab?.proxyUrl || "");
    setPinchTabTimezone(profile.backend?.pinchtab?.timezone || "");
    setPinchTabCloudEnabled(profile.backend?.pinchtab?.cloud?.enabled ?? false);
    setPinchTabCloudBucket(
      profile.backend?.pinchtab?.cloud?.bucket ||
        "conthunt-dev-pinchtab-profiles",
    );
    setPinchTabCloudPrefix(
      profile.backend?.pinchtab?.cloud?.prefix || "pinchtab/profiles",
    );
    setPinchTabCloudCredentialPath(
      profile.backend?.pinchtab?.cloud?.credentialPath || "",
    );
    setPinchTabKeepLocalCache(
      profile.backend?.pinchtab?.cloud?.keepLocalCache ?? true,
    );
    setSteelProxyUrl(profile.backend?.steel?.proxyUrl || "");
    setSteelExtensionPaths(profile.backend?.steel?.extensionPaths || []);
    setCloakBaseUrl(profile.backend?.cloak?.baseUrl || "http://127.0.0.1:8080");
    setCloakProxyUrl(profile.backend?.cloak?.proxyUrl || "");
    setCloakTimezone(profile.backend?.cloak?.timezone || "");
    setCloakLocale(profile.backend?.cloak?.locale || "");
    setCloakLaunchArgs(profile.backend?.cloak?.launchArgs || []);
    setCloakHeadless(profile.backend?.cloak?.headless ?? true);
    setCloakHumanize(profile.backend?.cloak?.humanize ?? true);
    setCloakGeoip(profile.backend?.cloak?.geoip ?? false);
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
        : profile.backend?.kind === "cloak"
          ? {
              kind: "cloak",
              cloak: {
                ...profile.backend?.cloak,
                baseUrl: cloakBaseUrl.trim() || undefined,
                proxyUrl: cloakProxyUrl.trim() || undefined,
                timezone: cloakTimezone.trim() || undefined,
                locale: cloakLocale.trim() || undefined,
                launchArgs:
                  cloakLaunchArgs.length > 0 ? cloakLaunchArgs : undefined,
                headless: cloakHeadless,
                humanize: cloakHumanize,
                geoip: cloakGeoip,
              },
            }
          : {
              kind: "pinchtab",
              ...(pinchTabProxyUrl.trim() || pinchTabTimezone.trim()
                ? {
                    pinchtab: {
                      proxyUrl: pinchTabProxyUrl.trim() || undefined,
                      timezone: pinchTabTimezone.trim() || undefined,
                      cloud: pinchTabCloudEnabled
                        ? {
                            enabled: true,
                            provider: "gcs",
                            bucket: pinchTabCloudBucket.trim() || undefined,
                            prefix: pinchTabCloudPrefix.trim() || undefined,
                            profileId:
                              profile.backend?.pinchtab?.cloud?.profileId,
                            credentialPath:
                              pinchTabCloudCredentialPath.trim() || undefined,
                            keepLocalCache: pinchTabKeepLocalCache,
                          }
                        : undefined,
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
    pinchTabCloudEnabled,
    pinchTabCloudBucket,
    pinchTabCloudPrefix,
    pinchTabCloudCredentialPath,
    pinchTabKeepLocalCache,
    steelProxyUrl,
    steelExtensionPaths,
    cloakBaseUrl,
    cloakProxyUrl,
    cloakTimezone,
    cloakLocale,
    cloakLaunchArgs,
    cloakHeadless,
    cloakHumanize,
    cloakGeoip,
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
      {profile.backend?.kind === "pinchtab" && (
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
          <div className="rounded border border-border-subtle bg-black/10 p-3">
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={pinchTabCloudEnabled}
                onChange={(e) => setPinchTabCloudEnabled(e.target.checked)}
              />
              Enable GCS cloud sync
            </label>
            {pinchTabCloudEnabled && (
              <div className="mt-3 flex flex-col gap-3">
                <Input
                  label="Bucket"
                  placeholder="conthunt-dev-pinchtab-profiles"
                  value={pinchTabCloudBucket}
                  onChange={(e) => setPinchTabCloudBucket(e.target.value)}
                />
                <Input
                  label="Prefix"
                  placeholder="pinchtab/profiles"
                  value={pinchTabCloudPrefix}
                  onChange={(e) => setPinchTabCloudPrefix(e.target.value)}
                />
                <Input
                  label="Credential path"
                  placeholder="/path/to/gcs-bucket-ops.json"
                  value={pinchTabCloudCredentialPath}
                  onChange={(e) =>
                    setPinchTabCloudCredentialPath(e.target.value)
                  }
                />
                <label className="flex items-center gap-2 text-sm text-text-primary">
                  <input
                    type="checkbox"
                    checked={pinchTabKeepLocalCache}
                    onChange={(e) =>
                      setPinchTabKeepLocalCache(e.target.checked)
                    }
                  />
                  Keep local cache after stop
                </label>
              </div>
            )}
          </div>
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
      {profile.backend?.kind === "cloak" && (
        <>
          <Input
            label="Cloak manager base URL"
            placeholder="http://127.0.0.1:8080"
            value={cloakBaseUrl}
            onChange={(e) => setCloakBaseUrl(e.target.value)}
          />
          <Input
            label="Proxy / IP (optional)"
            placeholder="http://user:pass@host:port"
            value={cloakProxyUrl}
            onChange={(e) => setCloakProxyUrl(e.target.value)}
          />
          <Input
            label="Timezone (optional)"
            placeholder="Australia/Sydney"
            value={cloakTimezone}
            onChange={(e) => setCloakTimezone(e.target.value)}
          />
          <Input
            label="Locale (optional)"
            placeholder="en-GB"
            value={cloakLocale}
            onChange={(e) => setCloakLocale(e.target.value)}
          />
          <Input
            label="Launch args (optional — comma-separated)"
            placeholder="--fingerprint-storage-quota=5000, --fingerprint-noise=false"
            value={listToCsv(cloakLaunchArgs)}
            onChange={(e) => setCloakLaunchArgs(csvToList(e.target.value))}
          />
          <div className="grid gap-2 sm:grid-cols-3">
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={cloakHeadless}
                onChange={(e) => setCloakHeadless(e.target.checked)}
              />
              Headless
            </label>
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={cloakHumanize}
                onChange={(e) => setCloakHumanize(e.target.checked)}
              />
              Humanize
            </label>
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={cloakGeoip}
                onChange={(e) => setCloakGeoip(e.target.checked)}
              />
              GeoIP
            </label>
          </div>
        </>
      )}
    </div>
  );
}
