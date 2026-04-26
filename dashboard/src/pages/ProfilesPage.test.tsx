import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import ProfilesPage from "./ProfilesPage";
import { useAppStore } from "../stores/useAppStore";
import type { Instance, Profile } from "../generated/types";
import * as api from "../services/api";

vi.mock("../services/api", () => ({
  fetchProfiles: vi.fn(),
  fetchProfileSync: vi.fn(),
  fetchProfileFinalize: vi.fn(),
  createProfile: vi.fn(),
  deleteProfile: vi.fn(),
  exportProfileConfigs: vi.fn(),
  updateProfile: vi.fn(),
  fetchInstances: vi.fn(),
  importProfileConfigs: vi.fn(),
  discoverCloudProfiles: vi.fn(),
  importCloudProfile: vi.fn(),
  launchInstance: vi.fn(),
  retryProfileFinalize: vi.fn(),
  discardProfileFinalize: vi.fn(),
  startProfileSync: vi.fn(),
  stopInstance: vi.fn(),
  fetchInstanceTabs: vi.fn(),
  fetchInstanceLogs: vi.fn(),
  fetchActivity: vi.fn(),
  fetchAllTabs: vi.fn(),
}));

const profiles: Profile[] = [
  {
    id: "prof_alpha",
    name: "alpha",
    created: "2026-03-01T10:00:00Z",
    lastUsed: "2026-03-05T10:00:00Z",
    diskUsage: 1024,
    sizeMB: 12,
    running: false,
    useWhen: "Use for personal logins",
    backend: {
      kind: "pinchtab",
      pinchtab: {
        cloud: {
          enabled: true,
          bucket: "bucket",
          prefix: "pinchtab/profiles",
          profileId: "cp_alpha",
        },
      },
    },
    cloudStatus: {
      state: "available",
    },
  },
  {
    id: "prof_beta",
    name: "beta",
    created: "2026-03-02T10:00:00Z",
    lastUsed: "2026-03-06T10:00:00Z",
    diskUsage: 2048,
    sizeMB: 24,
    running: true,
    accountEmail: "team@example.com",
    backend: {
      kind: "steel",
      steel: {
        proxyUrl: "http://proxy.local:8080",
        extensionPaths: ["/tmp/ext-one"],
      },
    },
  },
  {
    id: "prof_gamma",
    name: "gamma",
    created: "2026-03-03T10:00:00Z",
    lastUsed: "2026-03-07T10:00:00Z",
    diskUsage: 4096,
    sizeMB: 18,
    running: false,
    backend: {
      kind: "cloak",
      cloak: {
        baseUrl: "http://127.0.0.1:8080",
        profileId: "cloak-prof-gamma",
        proxyUrl: "http://proxy.old:8080",
        timezone: "UTC",
        launchArgs: ["--old-arg"],
        headless: false,
        humanize: false,
        geoip: false,
      },
    },
  },
];

const instances: Instance[] = [
  {
    id: "inst_beta",
    profileId: "prof_beta",
    profileName: "beta",
    port: "9988",
    headless: false,
    status: "running",
    startTime: "2026-03-06T10:00:00Z",
    attached: false,
  },
];

function renderProfilesPage() {
  return render(
    <MemoryRouter>
      <ProfilesPage />
    </MemoryRouter>,
  );
}

function clickSidebarProfile(name: string) {
  const profileNameEl = screen.getByText(name, {
    selector: ".text-sm.font-semibold",
  });
  const button = profileNameEl.closest("button") as HTMLElement;
  return userEvent.click(button);
}

function getDetailPanel() {
  return document.querySelector(
    ".dashboard-panel > .min-h-0.min-w-0.flex-1",
  ) as HTMLElement;
}

describe("ProfilesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchProfiles).mockResolvedValue(profiles);
    vi.mocked(api.fetchProfileSync).mockResolvedValue({
      state: "ready",
    });
    vi.mocked(api.fetchProfileFinalize).mockResolvedValue({
      state: "idle",
    });
    vi.mocked(api.fetchInstances).mockResolvedValue(instances);
    vi.mocked(api.exportProfileConfigs).mockResolvedValue({
      version: "pinchtab.profile-config.v1",
      exportedAt: "2026-04-25T00:00:00Z",
      profiles: [],
    });
    vi.mocked(api.importProfileConfigs).mockResolvedValue({
      status: "imported",
      profiles: [],
      count: 0,
    });
    vi.mocked(api.discoverCloudProfiles).mockResolvedValue({
      profiles: [],
    });
    vi.mocked(api.importCloudProfile).mockResolvedValue({
      status: "imported",
      name: "alpha-cloud",
    });
    vi.mocked(api.retryProfileFinalize).mockResolvedValue({
      state: "queued",
    });
    vi.mocked(api.discardProfileFinalize).mockResolvedValue({
      status: "discarded",
      name: "alpha",
    });
    vi.mocked(api.startProfileSync).mockResolvedValue({
      state: "queued",
    });
    useAppStore.setState({
      profiles,
      profilesLoading: false,
      instances,
    });
  });

  it("moves the running profile to the top and auto-selects it", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    const sidebar = document.querySelector(
      ".bg-bg-surface\\/50",
    ) as HTMLElement;
    const profileButtons = within(sidebar)
      .getAllByRole("button")
      .filter((button) =>
        ["alpha", "beta", "gamma"].some((name) =>
          button.textContent?.includes(name),
        ),
      );
    expect(profileButtons[0]).toHaveTextContent("beta");
    expect(profileButtons[1]).toHaveTextContent("alpha");

    const detailPanel = getDetailPanel()!;
    expect(
      within(detailPanel).getAllByText("team@example.com").length,
    ).toBeGreaterThan(0);
    expect(within(detailPanel).getByText("running")).toBeInTheDocument();
    expect(within(detailPanel).getByText("9988")).toBeInTheDocument();
  });

  it("switches the right detail pane when selecting another profile", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("alpha");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: alpha/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    expect(
      within(detailPanel).getAllByText("Use for personal logins").length,
    ).toBeGreaterThan(0);
    expect(
      within(detailPanel).getByRole("button", { name: "Start" }),
    ).toBeInTheDocument();
  });

  it("enables save only after profile fields change", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("alpha");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: alpha/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    const saveButton = within(detailPanel).getByRole("button", {
      name: "Save",
    });
    const nameInput = within(detailPanel).getByDisplayValue("alpha");

    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "alpha-updated");

    expect(saveButton).toBeEnabled();
  });

  it("saves Steel proxy and extension edits", async () => {
    const { updateProfile } = await import("../services/api");
    vi.mocked(updateProfile).mockResolvedValue({
      status: "updated",
      id: "prof_beta",
      name: "beta",
    });

    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    const proxyInput = within(detailPanel).getByPlaceholderText(
      "http://user:pass@host:port",
    );
    const extensionsInput = within(detailPanel).getByPlaceholderText(
      "/path/to/ext-one, /path/to/ext-two",
    );
    const saveButton = within(detailPanel).getByRole("button", {
      name: "Save",
    });

    await userEvent.clear(proxyInput);
    await userEvent.type(proxyInput, "http://proxy2.local:9090");
    fireEvent.change(extensionsInput, {
      target: { value: "/tmp/ext-two, /tmp/ext-three" },
    });
    await userEvent.click(saveButton);

    await waitFor(() => {
      expect(updateProfile).toHaveBeenCalledWith("prof_beta", {
        name: undefined,
        useWhen: "",
        backend: {
          kind: "steel",
          steel: {
            proxyUrl: "http://proxy2.local:9090",
            extensionPaths: ["/tmp/ext-two", "/tmp/ext-three"],
          },
        },
      });
    });
  });

  it("starts cloud sync from the toolbar", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("alpha");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: alpha/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    const syncButton = within(detailPanel).getByRole("button", {
      name: "Sync",
    });

    await userEvent.click(syncButton);

    await waitFor(() => {
      expect(api.startProfileSync).toHaveBeenCalledWith("prof_alpha");
    });
  });

  it("saves PinchTab proxy and timezone edits", async () => {
    const { updateProfile } = await import("../services/api");
    vi.mocked(updateProfile).mockResolvedValue({
      status: "updated",
      id: "prof_alpha",
      name: "alpha",
    });

    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("alpha");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: alpha/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    const proxyInput = within(detailPanel).getByPlaceholderText(
      "http://user:pass@host:port",
    );
    const timezoneInput =
      within(detailPanel).getByPlaceholderText("Asia/Kolkata");
    const saveButton = within(detailPanel).getByRole("button", {
      name: "Save",
    });

    await userEvent.type(proxyInput, "http://proxy.local:3128");
    await userEvent.type(timezoneInput, "Asia/Singapore");
    await userEvent.click(saveButton);

    await waitFor(() => {
      expect(updateProfile).toHaveBeenCalledWith("prof_alpha", {
        name: undefined,
        useWhen: undefined,
        backend: {
          kind: "pinchtab",
          pinchtab: {
            proxyUrl: "http://proxy.local:3128",
            timezone: "Asia/Singapore",
            cloud: {
              enabled: true,
              provider: "gcs",
              bucket: "bucket",
              prefix: "pinchtab/profiles",
              profileId: "cp_alpha",
              credentialPath: undefined,
              keepLocalCache: true,
            },
          },
        },
      });
    });
  });

  it("saves Cloak proxy, timezone, and launch arg edits", async () => {
    const { updateProfile } = await import("../services/api");
    vi.mocked(updateProfile).mockResolvedValue({
      status: "updated",
      id: "prof_gamma",
      name: "gamma",
    });

    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("gamma");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: gamma/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    const baseUrlInput = within(detailPanel).getByPlaceholderText(
      "http://127.0.0.1:8080",
    );
    const proxyInput = within(detailPanel).getByDisplayValue(
      "http://proxy.old:8080",
    );
    const timezoneInput = within(detailPanel).getByDisplayValue("UTC");
    const launchArgsInput = within(detailPanel).getByDisplayValue("--old-arg");
    const geoipCheckbox = within(detailPanel).getByLabelText("GeoIP");
    const saveButton = within(detailPanel).getByRole("button", {
      name: "Save",
    });

    await userEvent.clear(baseUrlInput);
    await userEvent.type(baseUrlInput, "http://127.0.0.1:8081");
    await userEvent.clear(proxyInput);
    await userEvent.type(proxyInput, "http://proxy.new:9090");
    await userEvent.clear(timezoneInput);
    await userEvent.type(timezoneInput, "Australia/Sydney");
    fireEvent.change(launchArgsInput, {
      target: {
        value: "--fingerprint-storage-quota=5000, --fingerprint-noise=false",
      },
    });
    await userEvent.click(geoipCheckbox);
    await userEvent.click(saveButton);

    await waitFor(() => {
      expect(updateProfile).toHaveBeenCalledWith("prof_gamma", {
        name: undefined,
        useWhen: "",
        backend: {
          kind: "cloak",
          cloak: {
            baseUrl: "http://127.0.0.1:8081",
            profileId: "cloak-prof-gamma",
            proxyUrl: "http://proxy.new:9090",
            timezone: "Australia/Sydney",
            locale: undefined,
            launchArgs: [
              "--fingerprint-storage-quota=5000",
              "--fingerprint-noise=false",
            ],
            headless: false,
            humanize: false,
            geoip: true,
          },
        },
      });
    });
  });

  it("exports selected profiles from the sidebar", async () => {
    const exportBundle = {
      version: "pinchtab.profile-config.v1",
      exportedAt: "2026-04-25T00:00:00Z",
      profiles: [
        {
          id: "prof_gamma",
          name: "gamma",
          backend: { kind: "cloak" as const },
        },
      ],
    };
    vi.mocked(api.exportProfileConfigs).mockResolvedValue(exportBundle);
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockReturnValue("blob:pinchtab-export");
    const revokeObjectURL = vi
      .spyOn(URL, "revokeObjectURL")
      .mockImplementation(() => {});
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});

    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select gamma for export"));
    await userEvent.click(
      screen.getByRole("button", { name: "Export Selected" }),
    );

    await waitFor(() => {
      expect(api.exportProfileConfigs).toHaveBeenCalledWith({
        ids: ["prof_gamma"],
      });
    });
    expect(clickSpy).toHaveBeenCalled();
    expect(createObjectURL).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:pinchtab-export");

    createObjectURL.mockRestore();
    revokeObjectURL.mockRestore();
    clickSpy.mockRestore();
  });

  it("imports profiles from a JSON bundle", async () => {
    const importedProfiles: Profile[] = [
      ...profiles,
      {
        id: "prof_layer_import",
        name: "Layer import",
        created: "2026-04-25T10:00:00Z",
        lastUsed: "2026-04-25T10:00:00Z",
        diskUsage: 512,
        sizeMB: 8,
        running: false,
        backend: {
          kind: "cloak",
          cloak: {
            baseUrl: "http://127.0.0.1:8080",
          },
        },
      },
    ];
    vi.mocked(api.importProfileConfigs).mockResolvedValue({
      status: "imported",
      profiles: ["Layer import"],
      count: 1,
    });
    vi.mocked(api.fetchProfiles)
      .mockResolvedValueOnce(profiles)
      .mockResolvedValueOnce(importedProfiles);

    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    const file = new File(
      [
        JSON.stringify({
          version: "pinchtab.profile-config.v1",
          exportedAt: "2026-04-25T00:00:00Z",
          profiles: [{ name: "Layer import", backend: { kind: "cloak" } }],
        }),
      ],
      "layer-import.json",
      { type: "application/json" },
    );

    await userEvent.upload(fileInput, file);

    await waitFor(() => {
      expect(api.importProfileConfigs).toHaveBeenCalledWith({
        bundle: {
          version: "pinchtab.profile-config.v1",
          exportedAt: "2026-04-25T00:00:00Z",
          profiles: [{ name: "Layer import", backend: { kind: "cloak" } }],
        },
      });
    });
    await waitFor(() => {
      expect(screen.getByText("Imported 1 profile.")).toBeInTheDocument();
    });
  });

  it("discovers and attaches a cloud profile", async () => {
    const cloudAttachedProfiles: Profile[] = [
      ...profiles,
      {
        id: "prof_layer_cloud",
        name: "Layer Headed",
        created: "2026-04-25T10:00:00Z",
        lastUsed: "2026-04-25T10:00:00Z",
        diskUsage: 1024,
        sizeMB: 10,
        running: false,
        backend: {
          kind: "pinchtab",
          pinchtab: {
            cloud: {
              enabled: true,
              bucket: "conthunt-dev-pinchtab-profiles",
              prefix: "pinchtab/profiles",
              profileId: "cp_remote_layer",
              credentialPath:
                "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json",
            },
          },
        },
      },
    ];
    vi.mocked(api.discoverCloudProfiles).mockResolvedValue({
      profiles: [
        {
          profileId: "cp_remote_layer",
          name: "Layer Headed",
          timezone: "America/Denver",
          locale: "en-US",
          updatedAt: "2026-04-26T10:00:00Z",
        },
      ],
    });
    vi.mocked(api.importCloudProfile).mockResolvedValue({
      status: "imported",
      name: "Layer Headed",
    });
    vi.mocked(api.fetchProfiles)
      .mockResolvedValueOnce(profiles)
      .mockResolvedValueOnce(cloudAttachedProfiles);

    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole("button", { name: "Import Cloud" }));
    await userEvent.click(screen.getByRole("button", { name: "Scan Cloud" }));

    await waitFor(() => {
      expect(api.discoverCloudProfiles).toHaveBeenCalledWith({
        bucket: "conthunt-dev-pinchtab-profiles",
        prefix: "pinchtab/profiles",
        credentialPath: "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json",
      });
    });

    const attachButton = await screen.findByRole("button", { name: "Attach" });
    await userEvent.click(attachButton);

    await waitFor(() => {
      expect(api.importCloudProfile).toHaveBeenCalledWith({
        bucket: "conthunt-dev-pinchtab-profiles",
        prefix: "pinchtab/profiles",
        credentialPath: "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json",
        profileId: "cp_remote_layer",
      });
    });
    await waitFor(() => {
      expect(api.fetchProfiles).toHaveBeenCalledTimes(1);
    });
  });
});
