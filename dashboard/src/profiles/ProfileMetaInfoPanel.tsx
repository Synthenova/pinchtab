import type { Profile, Instance } from "../generated/types";

interface Props {
  profile: Profile;
  instance?: Instance;
}

function MetaBlock({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border-subtle bg-black/10 p-4">
      <div className="dashboard-section-title mb-2 text-[0.68rem]">{label}</div>
      {children}
    </div>
  );
}

export default function ProfileMetaInfoPanel({ profile, instance }: Props) {
  const accountText = profile.accountEmail || profile.accountName || "";
  const sizeText = profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : "—";
  const browserType = instance?.attached
    ? "Attached via CDP"
    : instance?.headless
      ? "Headless"
      : "Headed";
  const backendText =
    profile.backend?.kind === "steel"
      ? "Steel Browser"
      : profile.backend?.kind === "cloak"
        ? "Cloak Manager"
        : "PinchTab";

  return (
    <MetaBlock label="Profile panel">
      <div className="space-y-3 text-sm text-text-secondary">
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">Status</span>
          <span className="text-right">{instance?.status || "stopped"}</span>
        </div>
        {instance?.port && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">Port</span>
            <span className="text-right">{instance.port}</span>
          </div>
        )}
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">
            Browser
          </span>
          <span className="text-right">{browserType}</span>
        </div>
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">
            Backend
          </span>
          <span className="text-right">{backendText}</span>
        </div>
        {profile.backend?.kind === "pinchtab" &&
          profile.backend?.pinchtab?.proxyUrl && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Proxy
              </div>
              <code className="dashboard-mono block break-all text-xs text-text-secondary">
                {profile.backend.pinchtab.proxyUrl}
              </code>
            </div>
          )}
        {profile.backend?.kind === "pinchtab" &&
          profile.backend?.pinchtab?.timezone && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Timezone
              </div>
              <code className="dashboard-mono block break-all text-xs text-text-secondary">
                {profile.backend.pinchtab.timezone}
              </code>
            </div>
          )}
        {profile.backend?.kind === "pinchtab" &&
          profile.backend?.pinchtab?.cloud && (
            <>
              <div className="flex items-center justify-between gap-3">
                <span className="dashboard-section-title text-[0.68rem]">
                  Cloud Sync
                </span>
                <span className="text-right">
                  {profile.cloudStatus?.state || "enabled"}
                </span>
              </div>
              {profile.backend.pinchtab.cloud.profileId && (
                <div>
                  <div className="dashboard-section-title mb-1 text-[0.68rem]">
                    Cloud Profile ID
                  </div>
                  <code className="dashboard-mono block break-all text-xs text-text-secondary">
                    {profile.backend.pinchtab.cloud.profileId}
                  </code>
                </div>
              )}
              {profile.backend.pinchtab.cloud.bucket && (
                <div>
                  <div className="dashboard-section-title mb-1 text-[0.68rem]">
                    Cloud Bucket
                  </div>
                  <code className="dashboard-mono block break-all text-xs text-text-secondary">
                    {profile.backend.pinchtab.cloud.bucket}
                    {profile.backend.pinchtab.cloud.prefix
                      ? `/${profile.backend.pinchtab.cloud.prefix}`
                      : ""}
                  </code>
                </div>
              )}
              {profile.cloudStatus?.leaseMachine && (
                <div className="flex items-center justify-between gap-3">
                  <span className="dashboard-section-title text-[0.68rem]">
                    Lease Holder
                  </span>
                  <span className="text-right">
                    {profile.cloudStatus.leaseUser
                      ? `${profile.cloudStatus.leaseUser} @ ${profile.cloudStatus.leaseMachine}`
                      : profile.cloudStatus.leaseMachine}
                  </span>
                </div>
              )}
              {profile.cloudStatus?.remoteVersion && (
                <div className="flex items-center justify-between gap-3">
                  <span className="dashboard-section-title text-[0.68rem]">
                    Remote Version
                  </span>
                  <span className="text-right">
                    {profile.cloudStatus.remoteVersion}
                  </span>
                </div>
              )}
              {profile.cloudStatus?.localVersion && (
                <div className="flex items-center justify-between gap-3">
                  <span className="dashboard-section-title text-[0.68rem]">
                    Local Version
                  </span>
                  <span className="text-right">
                    {profile.cloudStatus.localVersion}
                  </span>
                </div>
              )}
              {profile.cloudStatus?.message && (
                <div className="text-xs text-destructive">
                  {profile.cloudStatus.message}
                </div>
              )}
              {["queued", "checking", "downloading", "extracting"].includes(
                profile.cloudStatus?.state || "",
              ) && (
                <div className="text-xs text-text-muted">
                  Sync is currently in progress. You can keep this panel open
                  and use the Sync button to refresh state through the API.
                </div>
              )}
            </>
          )}
        {profile.backend?.kind === "steel" &&
          profile.backend.steel?.proxyUrl && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Proxy
              </div>
              <code className="dashboard-mono block break-all text-xs text-text-secondary">
                {profile.backend.steel.proxyUrl}
              </code>
            </div>
          )}
        {profile.backend?.kind === "steel" &&
          profile.backend.steel?.extensionPaths &&
          profile.backend.steel.extensionPaths.length > 0 && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Extensions
              </div>
              <div className="space-y-1">
                {profile.backend.steel.extensionPaths.map((path) => (
                  <code
                    key={path}
                    className="dashboard-mono block break-all text-xs text-text-secondary"
                  >
                    {path}
                  </code>
                ))}
              </div>
            </div>
          )}
        {profile.backend?.kind === "cloak" &&
          profile.backend.cloak?.proxyUrl && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Proxy
              </div>
              <code className="dashboard-mono block break-all text-xs text-text-secondary">
                {profile.backend.cloak.proxyUrl}
              </code>
            </div>
          )}
        {profile.backend?.kind === "cloak" &&
          profile.backend.cloak?.timezone && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Timezone
              </div>
              <code className="dashboard-mono block break-all text-xs text-text-secondary">
                {profile.backend.cloak.timezone}
              </code>
            </div>
          )}
        {profile.backend?.kind === "cloak" && profile.backend.cloak?.locale && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              Locale
            </div>
            <code className="dashboard-mono block break-all text-xs text-text-secondary">
              {profile.backend.cloak.locale}
            </code>
          </div>
        )}
        {profile.backend?.kind === "cloak" &&
          profile.backend.cloak?.launchArgs &&
          profile.backend.cloak.launchArgs.length > 0 && (
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                Launch Args
              </div>
              <div className="space-y-1">
                {profile.backend.cloak.launchArgs.map((arg) => (
                  <code
                    key={arg}
                    className="dashboard-mono block break-all text-xs text-text-secondary"
                  >
                    {arg}
                  </code>
                ))}
              </div>
            </div>
          )}
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">Size</span>
          <span className="text-right">{sizeText}</span>
        </div>
        {accountText && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              Account
            </span>
            <span className="text-right">{accountText}</span>
          </div>
        )}
        {profile.chromeProfileName && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              Identity
            </span>
            <span className="text-right">{profile.chromeProfileName}</span>
          </div>
        )}
        {instance?.attached && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              Connection
            </span>
            <span className="text-right">CDP attached</span>
          </div>
        )}
        {instance?.cdpUrl && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              CDP URL
            </div>
            <code className="dashboard-mono block break-all text-xs text-text-secondary">
              {instance.cdpUrl}
            </code>
          </div>
        )}
        {profile.path && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              Path
            </div>
            <code
              className={`dashboard-mono block break-all text-xs ${
                profile.pathExists ? "text-text-secondary" : "text-destructive"
              }`}
            >
              {profile.path}
              {!profile.pathExists && " (not found)"}
            </code>
          </div>
        )}
      </div>
    </MetaBlock>
  );
}
