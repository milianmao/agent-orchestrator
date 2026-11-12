import { appI18n } from "../../i18n";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { CodexMCPSection } from "./CodexMCPSection";

const { get, post, patch, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), patch: vi.fn(), remove: vi.fn() }));
vi.mock("../../lib/api-client", () => ({ apiClient: { GET: get, POST: post, PATCH: patch, DELETE: remove } }));
beforeEach(() => {
	vi.clearAllMocks();
	get.mockResolvedValue({ data: { servers: [{ name: "tools", transport: "stdio", enabled: true, command: "mcp", envRefs: { TOKEN: "MCP_TOKEN" }, missingReferences: ["MCP_TOKEN"] }] } });
	post.mockResolvedValue({ data: { newSessionsOnly: true } }); patch.mockResolvedValue({ data: { newSessionsOnly: true } });
});

it("显式禁用并提示新建会话生效，显示缺失引用", async () => {
	render(<CodexMCPSection />);
	expect(await screen.findByText(appI18n.t("codexMcp.missingReferences", { names: "MCP_TOKEN" }))).toBeInTheDocument();
	await userEvent.click(screen.getByRole("button", { name: `${appI18n.t("codexMcp.disable")} tools` }));
	expect(patch).toHaveBeenCalledWith("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: "tools" } }, body: { enabled: false } });
	expect(await screen.findByText(appI18n.t("codexMcp.newSessionsOnly"))).toBeInTheDocument();
});

it.each([false, true])("保存失败保留表单且隐藏原始错误（异常：%s）", async (reject) => {
	if (reject) post.mockRejectedValue(new Error("provider-secret-test-value"));
	else post.mockResolvedValue({ error: { code: "FORBIDDEN", message: "provider-secret-test-value" } });
	render(<CodexMCPSection />);
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.name")), "new-tools");
	await userEvent.type(screen.getByLabelText(appI18n.t("codexMcp.command")), "mcp-command");
	await userEvent.click(screen.getByRole("button", { name: appI18n.t("codexMcp.save") }));
	expect(await screen.findByRole("alert")).toHaveTextContent(appI18n.t("codexMcp.failed"));
	expect(screen.queryByText(/provider-secret-test-value/)).not.toBeInTheDocument();
	expect(screen.getByLabelText(appI18n.t("codexMcp.name"))).toHaveValue("new-tools");
	await waitFor(() => expect(screen.getByRole("button", { name: appI18n.t("codexMcp.save") })).toBeEnabled());
});
