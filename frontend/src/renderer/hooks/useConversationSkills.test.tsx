import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import { useConversationSkills } from "./useConversation";

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("../lib/host-clients", () => ({ clientForSessionHost: () => ({ GET: get }) }));

it("exposes a controller readiness conflict separately from an empty skill catalog", async () => {
	get.mockResolvedValueOnce({ error: { code: "CHAT_CONTROLLER_NOT_READY", message: "Controller is not running" }, response: { status: 409 } });
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
	const { result } = renderHook(() => useConversationSkills("session-1", true), { wrapper });
	await waitFor(() => expect(result.current.errorCode).toBe("CHAT_CONTROLLER_NOT_READY"));
	get.mockResolvedValueOnce({ data: { skills: [] } });
	await act(async () => { result.current.retry(); });
	await waitFor(() => expect(result.current.loaded).toBe(true));
	// A successful empty catalog must clear the previous readiness failure.
	expect(result.current.errorCode).toBeUndefined();
	expect(result.current.skills).toEqual([]);
	client.clear();
});
