import { appI18n } from "../../i18n";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { CodexMCPSection } from "./CodexMCPSection";

const { get, post, patch, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), patch: vi.fn(), remove: vi.fn() }));
vi.mock("../../lib/api-client", async (importOriginal) => {
	const actual = await importOriginal<typeof import("../../lib/api-client")>();
	return { ...actual, apiClient: { GET: get, POST: post, PATCH: patch, DELETE: remove } };
});
const server = { name: "tools", transport: "stdio", enabled: true, command: "mcp", envRefs: { TOKEN: "MCP_TOKEN" }, missingReferences: ["MCP_TOKEN"] };
beforeEach(() => {
	vi.clearAllMocks();
	get.mockResolvedValue({ data: { servers: [server] } });
	post.mockResolvedValue({ data: { newSessionsOnly: true } });
	patch.mockResolvedValue({ data: { newSessionsOnly: true } });
	remove.mockResolvedValue({ data: { newSessionsOnly: true } });
});

it("explains the empty list and refreshes it visibly", async () => {
	get.mockResolvedValueOnce({ data: { servers: [] } });
	render(<CodexMCPSection />);
	expect(await screen.findByText(appI18n.t("codexMcp.empty"))).toBeInTheDocument();
	expect(screen.getByText(appI18n.t("codexMcp.emptyHelp"))).toBeInTheDocument();
	expect(screen.getByText(appI18n.t("codexMcp.secretHelp"))).toBeInTheDocument();
	await userEvent.click(screen.getByRole("button", { name: appI18n.t("codexMcp.refresh") }));
	expect(await screen.findByText(appI18n.t("codexMcp.refreshed"))).toBeInTheDocument();
	expect(get).toHaveBeenCalledTimes(2);
	expect(screen.queryByText(appI18n.t("codexMcp.newSessionsOnly"), { selector: "[role='status']" })).not.toBeInTheDocument();
});

it("shows a retryable read error instead of the empty state", async () => {
	get.mockResolvedValueOnce({ error: { code: "INTERNAL" } });
	render(<CodexMCPSection />);
	expect(await screen.findByText(appI18n.t("codexMcp.readFailed"))).toBeInTheDocument();
	expect(screen.queryByText(appI18n.t("codexMcp.empty"))).not.toBeInTheDocument();
});

it("fills both examples with the format-specific fields and clears stale values on switching", async () => {
	render(<CodexMCPSection />);
	await screen.findByText(appI18n.t("codexMcp.configured"));
	await userEvent.click(screen.getByRole("button", { name: appI18n.t("codexMcp.httpExample") }));
	expect(screen.getByLabelText(appI18n.t("codexMcp.url"))).toHaveValue("https://mcp.example.com/mcp");
	expect(screen.getByLabelText(appI18n.t("codexMcp.headerRefs"))).toHaveValue(JSON.stringify({ Authorization: "MCP_AUTHORIZATION" }, null, 2));
	await userEvent.selectOptions(screen.getByLabelText(appI18n.t("codexMcp.transport")), "stdio");
	expect(screen.getByLabelText(appI18n.t("codexMcp.command"))).toHaveValue("");
	expect(screen.getByLabelText(appI18n.t("codexMcp.args"))).toHaveValue("[]");
	expect(screen.getByLabelText(appI18n.t("codexMcp.envRefs"))).toHaveValue("{}");
});

it("submits only the selected transport and keeps a failed form intact", async () => {
	post.mockResolvedValueOnce({ error: { code: "UNEXPECTED", message: "provider-secret-test-value" } });
	render(<CodexMCPSection />);
	await screen.findByText(appI18n.t("codexMcp.configured"));
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.name")), "new-tools");
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.command")), "mcp-command");
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	expect(await screen.findByText(appI18n.t("codexMcp.failed"))).toBeInTheDocument();
	expect(screen.queryByText(/provider-secret-test-value/)).not.toBeInTheDocument();
	expect(screen.getByLabelText(appI18n.t("codexMcp.name"))).toHaveValue("new-tools");
	expect(post).toHaveBeenCalledWith("/api/v1/settings/codex/mcp", { body: expect.objectContaining({ name: "new-tools", transport: "stdio", command: "mcp-command", args: [], url: "", envRefs: {}, headerRefs: {} }) });
});

it("shows local validation errors for JSON and references", async () => {
	render(<CodexMCPSection />);
	await screen.findByText(appI18n.t("codexMcp.configured"));
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.name")), "new-tools");
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.command")), "mcp-command");
	fireEvent.change(screen.getByLabelText(appI18n.t("codexMcp.args")), { target: { value: "{" } });
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	expect(await screen.findByText(appI18n.t("codexMcp.invalidJson"))).toBeInTheDocument();
	fireEvent.change(screen.getByLabelText(appI18n.t("codexMcp.args")), { target: { value: "{}" } });
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	expect(await screen.findByText(appI18n.t("codexMcp.invalidShape"))).toBeInTheDocument();
	fireEvent.change(screen.getByLabelText(appI18n.t("codexMcp.args")), { target: { value: "[]" } });
	fireEvent.change(screen.getByLabelText(appI18n.t("codexMcp.envRefs")), { target: { value: JSON.stringify({ TOKEN: "secret value" }) } });
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	expect(await screen.findByText(appI18n.t("codexMcp.invalidReference"))).toBeInTheDocument();
});

it("submits an HTTP example without stdio fields", async () => {
	render(<CodexMCPSection />);
	await screen.findByText(appI18n.t("codexMcp.configured"));
	await userEvent.click(screen.getByRole("button", { name: appI18n.t("codexMcp.httpExample") }));
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	await waitFor(() => expect(post).toHaveBeenCalledWith("/api/v1/settings/codex/mcp", { body: expect.objectContaining({ name: "remote-tools", transport: "http", command: "", args: [], url: "https://mcp.example.com/mcp", envRefs: {}, headerRefs: { Authorization: "MCP_AUTHORIZATION" } }) }));
});

it("keeps the list and lets the user retry after a read error", async () => {
	get.mockResolvedValueOnce({ error: { code: "INTERNAL" } }).mockResolvedValueOnce({ data: { servers: [server] } });
	render(<CodexMCPSection />);
	expect(await screen.findByText(appI18n.t("codexMcp.readFailed"))).toBeInTheDocument();
	expect(screen.queryByRole("button", { name: `${appI18n.t("codexMcp.edit")} tools` })).not.toBeInTheDocument();
	await userEvent.click(screen.getByRole("button", { name: appI18n.t("codexMcp.refresh") }));
	expect(await screen.findByRole("button", { name: `${appI18n.t("codexMcp.edit")} tools` })).toBeEnabled();
});

it("reports success and reloads after enable, disable and delete", async () => {
	render(<CodexMCPSection />);
	await screen.findByRole("button", { name: `${appI18n.t("codexMcp.disable")} tools` });
	await userEvent.click(screen.getByRole("button", { name: `${appI18n.t("codexMcp.disable")} tools` }));
	expect(await screen.findByText(appI18n.t("codexMcp.disabledSuccess"))).toBeInTheDocument();
	expect(patch).toHaveBeenCalledWith("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: "tools" } }, body: { enabled: false } });
	await userEvent.click(screen.getByRole("button", { name: `${appI18n.t("codexMcp.delete")} tools` }));
	expect(await screen.findByText(appI18n.t("codexMcp.deleted"))).toBeInTheDocument();
});

it("keeps save success visible when list refresh fails", async () => {
	get.mockResolvedValueOnce({ data: { servers: [] } }).mockResolvedValueOnce({ error: { code: "INTERNAL" } });
	render(<CodexMCPSection />);
	await screen.findByText(appI18n.t("codexMcp.empty"));
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.name")), "new-tools");
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.command")), "mcp-command");
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	expect(await screen.findByText(appI18n.t("codexMcp.saved"))).toBeInTheDocument();
	expect(await screen.findByText(appI18n.t("codexMcp.readFailed"))).toBeInTheDocument();
	expect(screen.getByLabelText(appI18n.t("codexMcp.name"))).toHaveValue("");
	await waitFor(() => expect(screen.getByRole("button", { name: appI18n.t("codexMcp.save") })).toBeEnabled());
});

it("surfaces known API errors without exposing the server message", async () => {
	post.mockResolvedValueOnce({ error: { code: "MCP_CONFIG_INVALID", message: "provider-secret-test-value" } });
	render(<CodexMCPSection />);
	await screen.findByText(appI18n.t("codexMcp.configured"));
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.name")), "new-tools");
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.command")), "mcp-command");
	fireEvent.submit(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }).closest("form")!);
	expect(await screen.findByText(appI18n.t("codexMcp.invalidConfig"))).toBeInTheDocument();
	expect(screen.queryByText(/provider-secret-test-value/)).not.toBeInTheDocument();
});
