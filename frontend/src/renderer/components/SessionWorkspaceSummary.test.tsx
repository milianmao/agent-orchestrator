import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { SessionWorkspaceSummary } from "./SessionWorkspaceSummary";
import { SessionArchiveDialog } from "./SessionArchiveDialog";
import type { WorkspaceSession } from "../types/workspace";

it("identifies external ownership in the inspector and promises to preserve the directory on archive", () => {
	const session = { workspaceMode: "project", workspaceOwnership: "external", artifacts: [] } as unknown as WorkspaceSession;
	render(<><SessionWorkspaceSummary session={session} />
		<SessionArchiveDialog open session={session} onConfirm={vi.fn()} onOpenChange={vi.fn()} trigger={<button>Archive</button>} /></>);
	expect(screen.getByText(/External project directory/)).toBeInTheDocument();
	expect(screen.getByText(/The project directory, current branch and uncommitted files are preserved/)).toBeInTheDocument();
});

it("labels legacy sessions as AO-owned worktrees", () => {
	render(<SessionWorkspaceSummary session={{}} />);
	expect(screen.getByText(/AO-owned worktree/)).toBeInTheDocument();
});
