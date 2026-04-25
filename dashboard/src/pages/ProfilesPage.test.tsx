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
  createProfile: vi.fn(),
  deleteProfile: vi.fn(),
  updateProfile: vi.fn(),
  fetchInstances: vi.fn(),
  launchInstance: vi.fn(),
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
    ".dashboard-panel .min-w-0.flex-1",
  ) as HTMLElement;
}

describe("ProfilesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchProfiles).mockResolvedValue(profiles);
    vi.mocked(api.fetchInstances).mockResolvedValue(instances);
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
    const sidebarButtons = within(sidebar).getAllByRole("button");
    const profileButtons = sidebarButtons.filter((b) =>
      b.classList.contains("border-b"),
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

    expect(saveButton).toBeDisabled();

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
});
