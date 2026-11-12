import { useTranslation } from "react-i18next";
import { useEffect, useState } from "react";
import type { components } from "../../../api/schema";
import { apiClient, apiErrorMessage } from "../../lib/api-client";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { SettingsSection } from "./SettingsSection";

type Config = components["schemas"]["CreateCodexMCPRequest"];
type Status = components["schemas"]["CodexMCPListResponse"]["servers"][number];
const blank: Config = { name: "", transport: "stdio", enabled: true, command: "", args: [], envRefs: {}, headerRefs: {} };

/** Codex 专用引用配置表单；保存不改变运行中的会话，也不接收凭据值。 */
export function CodexMCPSection() {
	const { t } = useTranslation();
	const [servers, setServers] = useState<Status[]>([]);
	const [draft, setDraft] = useState<Config>({ ...blank });
	const [editing, setEditing] = useState(false);
	const [args, setArgs] = useState("[]");
	const [refs, setRefs] = useState("{}");
	const [pending, setPending] = useState(false);
	const [error, setError] = useState("");
	const [notice, setNotice] = useState("");
	async function refresh() {
		const result = await apiClient.GET("/api/v1/settings/codex/mcp");
		if (result.error) throw new Error(apiErrorMessage(result.error));
		setServers(result.data.servers);
	}
	useEffect(() => { void refresh().catch(() => setError(t("codexMcp.readFailed"))); }, []);
	async function mutate(action: () => Promise<void>) {
		setPending(true); setError(""); setNotice("");
		try { await action(); await refresh(); setNotice(t("codexMcp.newSessionsOnly")); }
		catch (err) { setError(err instanceof Error ? err.message : t("codexMcp.failed")); }
		finally { setPending(false); }
	}
	function edit(server: Status) {
		const { missingReferences: _, startupFailure: _failure, ...config } = server;
		setDraft(config); setEditing(true); setArgs(JSON.stringify(config.args ?? []));
		setRefs(JSON.stringify(config.transport === "http" ? config.headerRefs ?? {} : config.envRefs ?? {}));
	}
	async function save() {
		await mutate(async () => {
			let parsedArgs: unknown, parsedRefs: unknown;
			try { parsedArgs = JSON.parse(args); parsedRefs = JSON.parse(refs); } catch { throw new Error(t("codexMcp.invalidJson")); }
			if (!Array.isArray(parsedArgs) || !parsedArgs.every(v => typeof v === "string") || !parsedRefs || typeof parsedRefs !== "object" || Array.isArray(parsedRefs) || !Object.values(parsedRefs).every(v => typeof v === "string")) throw new Error(t("codexMcp.invalidShape"));
			const references = parsedRefs as Record<string, string>;
			const body: Config = { ...draft, command: draft.transport === "stdio" ? draft.command : "", args: draft.transport === "stdio" ? parsedArgs : [], url: draft.transport === "http" ? draft.url : "", envRefs: draft.transport === "stdio" ? references : {}, headerRefs: draft.transport === "http" ? references : {} };
			const { name: _, ...patch } = body;
			const result = editing
				? await apiClient.PATCH("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: draft.name } }, body: patch })
				: await apiClient.POST("/api/v1/settings/codex/mcp", { body });
			if (result.error) throw new Error(apiErrorMessage(result.error));
			setDraft({ ...blank }); setEditing(false); setArgs("[]"); setRefs("{}");
		});
	}
	return <SettingsSection title={t("codexMcp.title")} sectionId="codex-mcp">
		<p className="text-sm text-muted-foreground">{t("codexMcp.description")}</p>
		{error && <p role="alert">{error}</p>}{notice && <p role="status">{notice}</p>}
		<Button disabled={pending} onClick={() => void mutate(refresh)}>{t("codexMcp.refresh")}</Button>
		<ul>{servers.map(server => <li key={server.name} className="flex flex-wrap items-center gap-2 py-2">
			<span>{server.name} · {server.transport} · {server.enabled ? t("codexMcp.enabled") : t("codexMcp.disabled")}</span>
			{server.startupFailure && <span role="status">{t("codexMcp.startupFailed")}</span>}
			{server.missingReferences.length > 0 && <span role="status">{t("codexMcp.missingReferences", { names: server.missingReferences.join(", ") })}</span>}
			<Button disabled={pending} onClick={() => edit(server)}>{t("codexMcp.edit")} {server.name}</Button>
			<Button disabled={pending} onClick={() => void mutate(async () => { const r = await apiClient.PATCH("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: server.name } }, body: { enabled: !server.enabled } }); if (r.error) throw new Error(apiErrorMessage(r.error)); })}>{server.enabled ? t("codexMcp.disable") : t("codexMcp.enable")} {server.name}</Button>
			<Button disabled={pending} onClick={() => void mutate(async () => { const r = await apiClient.DELETE("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: server.name } } }); if (r.error) throw new Error(apiErrorMessage(r.error)); })}>{t("codexMcp.delete")} {server.name}</Button>
		</li>)}</ul>
		<form className="grid gap-3" onSubmit={event => { event.preventDefault(); void save(); }}>
			<label>{t("codexMcp.name")}<Input required disabled={editing || pending} value={draft.name} onChange={e => setDraft({ ...draft, name: e.target.value })} /></label>
			<label>{t("codexMcp.transport")}<select disabled={pending} value={draft.transport} onChange={e => setDraft({ ...draft, transport: e.target.value })}><option value="stdio">{t("codexMcp.stdio")}</option><option value="http">{t("codexMcp.http")}</option></select></label>
			{draft.transport === "stdio" ? <><label>{t("codexMcp.command")}<Input required disabled={pending} value={draft.command ?? ""} onChange={e => setDraft({ ...draft, command: e.target.value })} /></label><label>{t("codexMcp.args")}<Input disabled={pending} value={args} onChange={e => setArgs(e.target.value)} /></label></> : <label>{t("codexMcp.url")}<Input required disabled={pending} value={draft.url ?? ""} onChange={e => setDraft({ ...draft, url: e.target.value })} /></label>}
			<label>{draft.transport === "stdio" ? "Env" : "Header"} {t("codexMcp.references")}<Input disabled={pending} value={refs} onChange={e => setRefs(e.target.value)} /></label>
			<label><input type="checkbox" disabled={pending} checked={draft.enabled} onChange={e => setDraft({ ...draft, enabled: e.target.checked })} />{t("codexMcp.enable")}</label>
			<Button type="submit" disabled={pending}>{pending ? t("codexMcp.saving") : t("codexMcp.save")}</Button>
			{editing && <Button type="button" disabled={pending} onClick={() => { setEditing(false); setDraft({ ...blank }); setArgs("[]"); setRefs("{}"); }}>{t("codexMcp.cancel")}</Button>}
		</form>
	</SettingsSection>;
}
