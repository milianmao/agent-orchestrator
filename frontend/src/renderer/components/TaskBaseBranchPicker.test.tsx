import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { TaskBaseBranchPicker } from "./TaskBaseBranchPicker";

it("switches workspace location, hides the base selector in project mode, and shows the lease conflict", async () => {
	const changeMode = vi.fn();
	const props = { value: "main", loading: false, error: false, disabled: false, onChange: vi.fn(), onWorkspaceModeChange: changeMode };
	const { rerender } = render(<TaskBaseBranchPicker {...props} workspaceMode="worktree" />);
	expect(screen.getByRole("button", { name: "Base branch" })).toBeInTheDocument();
	await userEvent.click(screen.getByRole("button", { name: "Workspace location" }));
	await userEvent.click(screen.getByRole("menuitem", { name: "Local project directory" }));
	expect(changeMode).toHaveBeenCalledWith("project");
	rerender(<TaskBaseBranchPicker {...props} workspaceMode="project" busy />);
	expect(screen.queryByRole("button", { name: "Base branch" })).not.toBeInTheDocument();
	expect(screen.getByRole("alert")).toHaveTextContent("Project directory is in use");
	rerender(<TaskBaseBranchPicker {...props} workspaceMode="worktree" />);
	expect(screen.getByRole("button", { name: "Base branch" })).toBeInTheDocument();
});

it("groups and searches existing local/origin branches and selects by keyboard", async () => {
	const changed = vi.fn();
	render(<TaskBaseBranchPicker value="main" loading={false} error={false} disabled={false} onChange={changed} data={{
		projectId: "proj", repository: "/repo", default: "main", truncated: false,
		branches: [
			{ name: "main", local: true, current: true, default: true },
			{ name: "origin/topic", remote: "origin", local: false, current: false, default: false },
			{ name: "upstream/other", remote: "upstream", local: false, current: false, default: false },
		],
	}} />);
	await userEvent.click(screen.getByRole("button", { name: "Base branch" }));
	expect(screen.getByRole("menuitem", { name: /main.*default/ })).toBeInTheDocument();
	expect(screen.queryByText("upstream/other")).not.toBeInTheDocument();
	const search = screen.getByRole("searchbox");
	await userEvent.type(search, "topic");
	expect(screen.queryByRole("menuitem", { name: /main/ })).not.toBeInTheDocument();
	fireEvent.keyDown(search, { key: "ArrowDown" });
	await userEvent.keyboard("{Enter}");
	expect(changed).toHaveBeenCalledWith("origin/topic");
});

it("focuses an unresolved branch and reports a read failure", () => {
	render(<TaskBaseBranchPicker value="" loading={false} error disabled={false} onChange={vi.fn()} />);
	expect(screen.getByRole("button", { name: "Base branch" })).toHaveFocus();
	expect(screen.getByRole("alert")).toBeInTheDocument();
});

it("shows loading and then an empty repository without offering an invented default", async () => {
	const props = { value: "", error: false, disabled: false, onChange: vi.fn() };
	const { rerender } = render(<TaskBaseBranchPicker {...props} loading />);
	await userEvent.click(screen.getByRole("button", { name: "Base branch" }));
	expect(screen.getByText("Loading branches…")).toBeInTheDocument();
	expect(screen.queryByRole("menuitem")).not.toBeInTheDocument();
	rerender(<TaskBaseBranchPicker {...props} loading={false} />);
	expect(screen.queryByText("Loading branches…")).not.toBeInTheDocument();
	expect(screen.queryByRole("menuitem")).not.toBeInTheDocument();
	expect(props.onChange).not.toHaveBeenCalled();
});
