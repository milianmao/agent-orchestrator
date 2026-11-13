import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import type { components } from "../../api/schema";
import { TRIGGER_CLASS } from "./chat/TurnSettingsBar";
import { SettingsOptionMenu } from "./settings/SettingsOptionMenu";

/** 本次任务选基只返回 seed，不编辑项目配置或最终任务分支。 */
export function TaskBaseBranchPicker({ value, data, loading, error, disabled, onChange, workspaceMode = "worktree", onWorkspaceModeChange, busy, workspaceError }: {
	workspaceMode?: "worktree" | "project";
	onWorkspaceModeChange?: (value: "worktree" | "project") => void;
	busy?: boolean;
	workspaceError?: string;
	value: string;
	data?: components["schemas"]["ProjectBranches"];
	loading: boolean;
	error: boolean;
	disabled: boolean;
	onChange: (value: string) => void;
}) {
	const { t } = useTranslation();
	const trigger = useRef<HTMLButtonElement>(null);
	useEffect(() => {
		if (workspaceMode === "worktree" && !value && !loading && !disabled) trigger.current?.focus();
	}, [value, loading, disabled, workspaceMode]);
	const branches = (data?.branches ?? []).filter((branch) => !branch.remote || branch.remote === "origin");
	const options = [false, true].flatMap((remote) => branches.filter((branch) => Boolean(branch.remote) === remote).map((branch) => ({
		value: branch.name,
		label: branch.name + (branch.default ? ` (${t("newTask.defaultBase")})` : ""),
		group: remote ? t("settings.project.defaultBranchRemoteGroup", { remote: "origin" }) : t("settings.project.defaultBranchLocalGroup"),
	})));
	return <>
		{onWorkspaceModeChange ? <SettingsOptionMenu value={workspaceMode}
			options={[{ value: "project", label: t("newTask.projectDirectory") }, { value: "worktree", label: t("newTask.newWorktree") }]}
			onChange={(value) => onWorkspaceModeChange(value as "worktree" | "project")} disabled={disabled}
			aria-label={t("newTask.workspaceLocation")} triggerClassName={TRIGGER_CLASS} menuAlign="start" />
			: <span className={TRIGGER_CLASS}>{t("newTask.worktree")}</span>}
		{busy && <span role="alert" className="text-xs text-warning">{t("newTask.projectDirectoryBusy")}</span>}
		{workspaceError && <span role="alert" className="text-xs text-warning">{workspaceError}</span>}
		{workspaceMode === "worktree" && <>
		<SettingsOptionMenu value={value} options={options} onChange={onChange} disabled={disabled}
			aria-label={t("newTask.baseBranch")} triggerRef={trigger} triggerClassName={TRIGGER_CLASS}
			placeholder={value || t("newTask.chooseBaseBranch")} searchable searchPlaceholder={t("newTask.searchBranches")}
			emptyLabel={loading ? t("newTask.loadingBranches") : error ? t("settings.project.defaultBranchLoadFailed") : t("settings.project.defaultBranchEmpty")}
			menuAlign="start" />
		{error && <span role="alert" className="text-xs text-warning">{t("settings.project.defaultBranchLoadFailed")}</span>}
		</>}
	</>;
}