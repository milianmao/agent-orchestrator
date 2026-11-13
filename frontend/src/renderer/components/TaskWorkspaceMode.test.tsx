import { act, cleanup, render, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), remove: vi.fn(), kind: "single_repo", busy: false, cloud: false }));
vi.mock("../lib/api-client", () => ({
	apiClient: { GET: h.get, POST: h.post, DELETE: h.remove },
	apiErrorCode: (e: { code?: string }) => e?.code,
	apiErrorMessage: (e: { message?: string }) => e?.message ?? "request failed",
}));
vi.mock("../hooks/useAgentReadinessQuery", async (original) => ({
	...await original<typeof import("../hooks/useAgentReadinessQuery")>(),
	useAgentReadinessQuery: () => ({ data: { agents: [agentReadiness("codex", "Codex")] }, isSuccess: true }),
	ensureAgentReadiness: vi.fn(),
}));
vi.mock("../hooks/useWorkspaceQuery", () => ({ useCloudProjectsQuery: () => ({ data: h.cloud ? [{ id: "p", config: {} }] : [] }), cloudSessionsQueryKey: ["cloud-sessions"], seedCreatedFreestyleSession: vi.fn() }));
vi.mock("../hooks/useCloudCp", () => ({ useCloudCp: () => ({ client: {}, baseUrl: "https://example.test" }) }));
vi.mock("../hooks/useCloudOrg", () => ({ useCloudOrg: () => ({}) }));
vi.mock("../hooks/useCloudOffering", () => ({ useCloudOffering: () => ({ chatHarnesses: [], loaded: false }) }));
vi.mock("../lib/telemetry", () => ({ captureRendererEvent: vi.fn() }));

import { useTaskLaunch } from "./TaskComposer";
import { agentReadiness } from "../test/agent-readiness-fixtures";

let task: ReturnType<typeof useTaskLaunch>;
function Launcher({ projectId = "p" }: { projectId?: string }) {
	task = useTaskLaunch({ projectId, chooseTaskBase: true });
	return null;
}
function mount(projectId?: string) {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	render(<QueryClientProvider client={client}><Launcher projectId={projectId} /></QueryClientProvider>);
}
beforeEach(() => {
	h.kind = "single_repo"; h.busy = false; h.cloud = false;
	h.get.mockImplementation(async (path: string) => path.endsWith("/branches")
		? { data: { default: "main", branches: [] } }
		: path.includes("/models") ? { data: { models: [], allowCustom: true, selectionMode: "text" } }
		: { data: { status: "ok", project: { id: "p", kind: h.kind, config: { worker: { agent: "codex" } } } } });
	h.post.mockImplementation(async (path: string, options: { body?: { workspaceMode?: string } }) => {
		if (path.endsWith("/prepare")) return options.body?.workspaceMode === "project"
			? h.busy ? { error: { code: "PROJECT_DIRECTORY_BUSY", message: "busy" } } : { data: { taskPreparation: "" } }
			: { data: { taskPreparation: "old-worktree" } };
		return { data: { workerId: "new-worker" } };
	});
	h.remove.mockResolvedValue({});
});
afterEach(() => { cleanup(); vi.clearAllMocks(); window.localStorage.removeItem("ao.taskComposer.preferences.v1"); });

it("cancels the old worktree preparation and sends project mode without branch or baseBranch", async () => {
	mount();
	await waitFor(() => expect(task.preparedSessionId).toBe("old-worktree"));
	act(() => task.setWorkspaceMode("project"));
	await waitFor(() => expect(task.preparedSessionId).toBe(""));
	expect(task.baseSelectable).toBe(false);
	expect(h.remove).toHaveBeenCalledWith("/api/v1/task-preparations/{token}", { params: { path: { token: "old-worktree" } } });
	await waitFor(() => expect(task.canSubmit).toBe(true));
	await act(async () => { await task.launch("Implement it"); });
	const payload = h.post.mock.calls.find(([path]) => path.endsWith("/delegate"))?.[1].body;
	expect(payload.workspaceMode).toBe("project");
	expect(payload.baseBranch).toBeUndefined();
	expect(payload.branch).toBeUndefined();
	expect(payload.taskPreparation).toBeUndefined();
	act(() => task.setWorkspaceMode("worktree"));
	expect(task.baseSelectable).toBe(true);
	expect(task.baseBranch).toBe("main");
});

it("blocks creation while the persisted project directory lease is busy", async () => {
	h.busy = true; mount();
	await waitFor(() => expect(task.workspaceSelectable).toBe(true));
	act(() => task.setWorkspaceMode("project"));
	await waitFor(() => expect(task.workspaceBusy).toBe(true));
	expect(task.canSubmit).toBe(false);
	await act(async () => { await expect(task.launch("Task")).rejects.toThrow(); });
	expect(h.post.mock.calls.some(([path]) => path.endsWith("/delegate"))).toBe(false);
});

it.each(["scratch", "workspace"])("does not offer project directory mode for %s", async (kind) => {
	h.kind = kind; mount();
	await waitFor(() => expect(task.canSubmit).toBe(true));
	act(() => task.setWorkspaceMode("project"));
	expect(task.workspaceSelectable).toBe(false);
	expect(task.workspaceMode).toBe("worktree");
});

it("does not offer project directory mode for standalone work", () => {
	mount("__standalone__");
	act(() => task.setWorkspaceMode("project"));
	expect(task.workspaceSelectable).toBe(false);
	expect(task.workspaceMode).toBe("worktree");
});

it("does not offer project directory mode for cloud work", () => {
	h.cloud = true;
	mount();
	act(() => task.setWorkspaceMode("project"));
	expect(task.workspaceSelectable).toBe(false);
	expect(task.workspaceMode).toBe("worktree");
});
