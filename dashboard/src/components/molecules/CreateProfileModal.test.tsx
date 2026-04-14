import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CreateProfileModal from "./CreateProfileModal";

vi.mock("../../services/api", () => ({
  createProfile: vi.fn(),
}));

describe("CreateProfileModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("submits the local form with the default PinchTab backend", async () => {
    const { createProfile } = await import("../../services/api");
    const onClose = vi.fn();
    const onCreated = vi.fn();

    vi.mocked(createProfile).mockResolvedValue({
      status: "ok",
      id: "prof_work",
      name: "work",
    });

    render(
      <CreateProfileModal
        open={true}
        onClose={onClose}
        onCreated={onCreated}
      />,
    );

    await userEvent.type(
      screen.getByPlaceholderText("e.g. personal, work, scraping"),
      "work",
    );
    await userEvent.type(
      screen.getByPlaceholderText(
        "e.g. I need to access Gmail for the team account",
      ),
      "Team account access",
    );

    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(createProfile).toHaveBeenCalledWith({
        name: "work",
        useWhen: "Team account access",
        backend: { kind: "pinchtab" },
      });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCreated).toHaveBeenCalledWith("prof_work");
  });

  it("submits a Steel-backed profile with an optional proxy URL", async () => {
    const { createProfile } = await import("../../services/api");
    const onClose = vi.fn();
    const onCreated = vi.fn();

    vi.mocked(createProfile).mockResolvedValue({
      status: "ok",
      id: "prof_steel",
      name: "steel-work",
    });

    render(
      <CreateProfileModal
        open={true}
        onClose={onClose}
        onCreated={onCreated}
      />,
    );

    await userEvent.type(
      screen.getByPlaceholderText("e.g. personal, work, scraping"),
      "steel-work",
    );
    await userEvent.selectOptions(screen.getByLabelText("Browser backend"), [
      "steel",
    ]);
    await userEvent.type(
      screen.getByPlaceholderText("http://user:pass@host:port"),
      "http://proxy.local:8080",
    );
    await userEvent.type(
      screen.getByPlaceholderText("/path/to/ext-one, /path/to/ext-two"),
      "/tmp/ext-one, /tmp/ext-two",
    );

    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(createProfile).toHaveBeenCalledWith({
        name: "steel-work",
        useWhen: undefined,
        backend: {
          kind: "steel",
          steel: {
            proxyUrl: "http://proxy.local:8080",
            extensionPaths: ["/tmp/ext-one", "/tmp/ext-two"],
          },
        },
      });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCreated).toHaveBeenCalledWith("prof_steel");
  });

  it("submits a PinchTab-backed profile with proxy and timezone", async () => {
    const { createProfile } = await import("../../services/api");
    const onClose = vi.fn();
    const onCreated = vi.fn();

    vi.mocked(createProfile).mockResolvedValue({
      status: "ok",
      id: "prof_pinchtab",
      name: "pinchtab-work",
    });

    render(
      <CreateProfileModal
        open={true}
        onClose={onClose}
        onCreated={onCreated}
      />,
    );

    await userEvent.type(
      screen.getByPlaceholderText("e.g. personal, work, scraping"),
      "pinchtab-work",
    );
    await userEvent.type(
      screen.getByPlaceholderText(
        "e.g. I need to access Gmail for the team account",
      ),
      "Personal browsing",
    );
    await userEvent.type(
      screen.getByPlaceholderText("http://user:pass@host:port"),
      "http://proxy.local:3128",
    );
    await userEvent.type(
      screen.getByPlaceholderText("Asia/Kolkata"),
      "Asia/Singapore",
    );

    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(createProfile).toHaveBeenCalledWith({
        name: "pinchtab-work",
        useWhen: "Personal browsing",
        backend: {
          kind: "pinchtab",
          pinchtab: {
            proxyUrl: "http://proxy.local:3128",
            timezone: "Asia/Singapore",
          },
        },
      });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCreated).toHaveBeenCalledWith("prof_pinchtab");
  });

  it("resets its local fields when closed and reopened", async () => {
    const { rerender } = render(
      <CreateProfileModal
        open={true}
        onClose={() => {}}
        onCreated={() => {}}
      />,
    );

    const nameInput = screen.getByPlaceholderText(
      "e.g. personal, work, scraping",
    );
    await userEvent.type(nameInput, "scratch");

    rerender(
      <CreateProfileModal
        open={false}
        onClose={() => {}}
        onCreated={() => {}}
      />,
    );

    rerender(
      <CreateProfileModal
        open={true}
        onClose={() => {}}
        onCreated={() => {}}
      />,
    );

    expect(
      screen.getByPlaceholderText("e.g. personal, work, scraping"),
    ).toHaveValue("");
    expect(screen.getByLabelText("Browser backend")).toHaveValue("pinchtab");
  });
});
