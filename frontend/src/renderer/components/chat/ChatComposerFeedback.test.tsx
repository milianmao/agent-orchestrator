import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { appI18n } from "../../i18n";
import { localPromptOptimizationRequest, usePromptOptimization } from "../../hooks/useConversationPromptSuggestion";
import { lexicalEditorText, typeInLexicalEditor } from "../../test/lexical";
import { TooltipProvider } from "../ui/tooltip";
import { ChatComposer } from "./ChatComposer";

const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock("../../lib/host-clients", () => ({ clientForSessionHost: () => ({ POST: post }) }));

function renderComposer(props: Partial<Parameters<typeof ChatComposer>[0]> = {}) {
	return render(<ChatComposer onSend={vi.fn()} onCompact={vi.fn()} {...props} />, { wrapper: TooltipProvider });
}

describe("slash catalog feedback", () => {
	it("labels AO commands separately from provider skills and preserves skill insertion", async () => {
		renderComposer({ skillsLoaded: true, skills: [{ name: "review", displayName: "Review", source: "repo" }] });
		const editor = screen.getByRole("combobox");
		await typeInLexicalEditor(editor, "/");
		const menu = await screen.findByRole("listbox");
		expect(within(menu).getByRole("option", { name: /compact/ })).toHaveTextContent(appI18n.t("chat.slashCatalog.aoCommand"));
		const skill = within(menu).getByRole("option", { name: /Review/ });
		expect(skill).toHaveTextContent(appI18n.t("chat.slashCatalog.agentSkill"));
		expect(skill).toHaveTextContent("repo");
		expect(within(menu).getAllByRole("option")).toHaveLength(2);
		await userEvent.click(skill);
		expect(lexicalEditorText(editor)).toBe("/review ");
	});

	it("explains an empty provider catalog even when compact is available", async () => {
		renderComposer({ skillsLoaded: true });
		await typeInLexicalEditor(screen.getByRole("combobox"), "/");
		expect(await screen.findByTestId("slash-catalog-status")).toHaveTextContent(appI18n.t("chat.slashCatalog.empty"));
		expect(screen.getByRole("option", { name: /compact/ })).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");
		expect(screen.queryByTestId("slash-catalog-status")).not.toBeInTheDocument();
	});

	it("keeps loading and readiness failures visible beside AO commands and allows retry", async () => {
		const retry = vi.fn();
		const view = renderComposer({ skillsLoaded: false, onRetrySkills: retry });
		await typeInLexicalEditor(screen.getByRole("combobox"), "/");
		expect(await screen.findByTestId("slash-catalog-status")).toHaveTextContent(appI18n.t("chat.slashCatalog.loading"));
		const reason = appI18n.t("chat.slashCatalog.controllerNotReady");
		view.rerender(<ChatComposer onSend={vi.fn()} onCompact={vi.fn()} skillsLoaded={false} skillsError={reason} onRetrySkills={retry} />);
		expect(screen.getByTestId("slash-catalog-status")).toHaveTextContent(reason);
		await userEvent.click(screen.getByRole("button", { name: appI18n.t("chat.slashCatalog.retry") }));
		expect(retry).toHaveBeenCalledOnce();
	});
});

describe("polish feedback", () => {
	it.each(["controller", "turn", "unavailable"] as const)("disables polishing with the %s reason", async (reason) => {
		const onOptimizeDraft = vi.fn();
		const message = appI18n.t(`chat.promptSuggestion.disabled.${reason}`);
		renderComposer({ onOptimizeDraft, optimizeDisabledReason: message });
		await typeInLexicalEditor(screen.getByRole("combobox"), "fix login");
		const button = screen.getByRole("button", { name: appI18n.t("chat.promptSuggestion.polishAction") });
		expect(button).toBeDisabled();
		expect(button).toHaveAccessibleDescription(message);
		expect(screen.getByRole("status")).toHaveTextContent(message);
		await userEvent.click(button);
		expect(onOptimizeDraft).not.toHaveBeenCalled();
	});

	it.each([
		["stale", { error: { code: "CHAT_PROMPT_SUGGESTION_STALE" }, response: { status: 409 } }],
		["tooLong", { error: { code: "CHAT_PROMPT_DRAFT_TOO_LONG" }, response: { status: 400 } }],
		["empty", { data: { turnId: "turn-1", prompt: "  " } }],
		["unavailable", { error: { code: "CHAT_PROMPT_SUGGESTION_UNAVAILABLE" }, response: { status: 503 } }],
	] as const)("shows actionable %s feedback from the API and keeps the draft", async (failure, response) => {
		post.mockResolvedValue(response);
		function Composer() {
			const optimization = usePromptOptimization({ enabled: true, request: localPromptOptimizationRequest() });
			return <ChatComposer onSend={vi.fn()} onOptimizeDraft={(draft) => optimization.optimize("session-1", "turn-1", draft)}
				optimizing={optimization.pending} optimizeFailure={optimization.failure} />;
		}
		render(<Composer />, { wrapper: TooltipProvider });
		const editor = screen.getByRole("combobox");
		await typeInLexicalEditor(editor, "fix login");
		const button = screen.getByRole("button", { name: appI18n.t("chat.promptSuggestion.polishAction") });
		await userEvent.click(button);
		await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent(appI18n.t(`chat.promptSuggestion.failure.${failure}`)));
		expect(button).toBeEnabled();
		expect(button).not.toHaveAttribute("aria-busy");
		expect(lexicalEditorText(editor)).toBe("fix login");
		expect(screen.queryByTestId("optimized-draft")).not.toBeInTheDocument();
	});
});
