import { useTranslation } from "react-i18next";
import type { WorkspaceSession } from "../types/workspace";

/** The inspector makes the directory's owner explicit before a user archives it. */
export function SessionWorkspaceSummary({ session }: { session: Pick<WorkspaceSession, "workspaceMode" | "workspaceOwnership"> }) {
	const { t } = useTranslation();
	const external = session.workspaceOwnership === "external" || session.workspaceMode === "project";
	return <p className="mt-2 text-xs text-muted-foreground">
		{t("newTask.workspaceLocation")}: {external ? t("inspector.workspaceExternal") : t("inspector.workspaceOwned")}
	</p>;
}
