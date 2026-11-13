import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import type { components } from "../../../api/schema";
import { apiClient, apiErrorCode } from "../../lib/api-client";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { SettingsSection } from "./SettingsSection";

type Config = components["schemas"]["CreateCodexMCPRequest"];
type Status = components["schemas"]["CodexMCPListResponse"]["servers"][number];
type Transport = Config["transport"];
const blank: Config = { name: "", transport: "stdio", enabled: true, command: "", args: [], url: "", envRefs: {}, headerRefs: {} };
const stdioArgs = ["-y", "@modelcontextprotocol/server-filesystem", "/absolute/path/to/folder"];
const httpUrl = "https://mcp.example.com/mcp";
const httpRefs = { Authorization: "MCP_AUTHORIZATION" };

/** 管理 Codex 的引用配置；区分读取与写入结果，不回显原始错误或修改当前会话。 */
export function CodexMCPSection() {
	const { t } = useTranslation();
	const id = useId();
	const nameInput = useRef<HTMLInputElement>(null);
	const [servers, setServers] = useState<Status[] | null>(null);
	const [draft, setDraft] = useState<Config>({ ...blank });
	const [editing, setEditing] = useState(false);
	const [args, setArgs] = useState("[]");
	const [refs, setRefs] = useState("{}");
	const [loading, setLoading] = useState(true);
	const [pending, setPending] = useState(false);
	const [listError, setListError] = useState("");
	const [error, setError] = useState("");
	const [notice, setNotice] = useState("");
	const busy = pending || loading;

	/** 读取失败保留上次列表；卸载或语言切换取消的请求不能覆盖新状态。 */
	const refresh = useCallback(async (signal?: AbortSignal): Promise<boolean> => {
		setLoading(true);
		setListError("");
		try {
			const result = await apiClient.GET("/api/v1/settings/codex/mcp", { signal });
			if (result.error || !result.data) throw result.error ?? new Error("MCP list response missing data");
			if (signal?.aborted) return false;
			setServers(result.data.servers);
			return true;
		} catch {
			if (!signal?.aborted) setListError(t("codexMcp.readFailed"));
			return false;
		} finally {
			if (!signal?.aborted) setLoading(false);
		}
	}, [t]);
	useEffect(() => {
		const controller = new AbortController();
		void refresh(controller.signal);
		return () => controller.abort();
	}, [refresh]);

	/** 仅将已知错误码转为可操作提示，避免服务器或网络异常泄露凭据。 */
	async function mutate(action: () => Promise<void>, success: string) {
		setPending(true); setError(""); setNotice("");
		try {
			await action();
			setNotice(success);
			await refresh();
		} catch (failure) {
			switch (apiErrorCode(failure)) {
				case "MCP_CONFIG_EXISTS": setError(t("codexMcp.exists")); break;
				case "MCP_CONFIG_NOT_FOUND": setError(t("codexMcp.notFound")); break;
				case "MCP_CONFIG_INVALID": setError(t("codexMcp.invalidConfig")); break;
				default: setError(t("codexMcp.failed"));
			}
		} finally {
			setPending(false);
		}
	}

	/** 编辑时只装载引用字段，状态信息不进入写入请求。 */
	function edit(server: Status) {
		const { missingReferences: _, startupFailure: _failure, ...config } = server;
		setDraft(config); setEditing(true); setArgs(JSON.stringify(config.args ?? []));
		setRefs(JSON.stringify(config.transport === "http" ? config.headerRefs ?? {} : config.envRefs ?? {}, null, 2));
		setError(""); setNotice("");
	}

	function reset() {
		setDraft({ ...blank }); setEditing(false); setArgs("[]"); setRefs("{}"); setError("");
	}

	/** 切换方式时清空另一方式的参数与引用，旧值不会混入新配置。 */
	function changeTransport(transport: Transport) {
		setDraft({ ...draft, transport, command: "", args: [], url: "", envRefs: {}, headerRefs: {} });
		setArgs("[]"); setRefs("{}"); setError("");
	}

	/** 示例只填充草稿，路径、域名与 secret 所在的进程环境仍需用户配置。 */
	function useExample(transport: Transport) {
		setDraft({ ...blank, name: transport === "stdio" ? "local-files" : "remote-tools", transport, command: transport === "stdio" ? "npx" : "", url: transport === "http" ? httpUrl : "" });
		setEditing(false); setArgs(JSON.stringify(transport === "stdio" ? stdioArgs : []));
		setRefs(JSON.stringify(transport === "http" ? httpRefs : {}, null, 2));
		setError(""); setNotice(""); nameInput.current?.focus();
	}

	/** 本地格式错误保留具体提示；仅提交对应方式的字段和引用名。 */
	async function save() {
		setError(""); setNotice("");
		let parsedArgs: unknown, parsedRefs: unknown;
		try {
			parsedArgs = draft.transport === "stdio" ? JSON.parse(args) : [];
			parsedRefs = JSON.parse(refs);
		} catch {
			setError(t("codexMcp.invalidJson")); return;
		}
		if (!Array.isArray(parsedArgs) || !parsedArgs.every(value => typeof value === "string") || !parsedRefs || typeof parsedRefs !== "object" || Array.isArray(parsedRefs) || !Object.values(parsedRefs).every(value => typeof value === "string")) {
			setError(t("codexMcp.invalidShape")); return;
		}
		const references = parsedRefs as Record<string, string>;
		if (!Object.values(references).every(value => /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(value))) {
			setError(t("codexMcp.invalidReference")); return;
		}
		const body: Config = { ...draft, command: draft.transport === "stdio" ? draft.command : "", args: draft.transport === "stdio" ? parsedArgs : [], url: draft.transport === "http" ? draft.url : "", envRefs: draft.transport === "stdio" ? references : {}, headerRefs: draft.transport === "http" ? references : {} };
		await mutate(async () => {
			const { name: _, ...patch } = body;
			const result = editing
				? await apiClient.PATCH("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: draft.name } }, body: patch })
				: await apiClient.POST("/api/v1/settings/codex/mcp", { body });
			if (result.error) throw result.error;
			reset();
		}, t("codexMcp.saved"));
	}

	return <SettingsSection title={t("codexMcp.title")} sectionId="codex-mcp">
		<div className="space-y-3 pb-6 text-sm">
			<p className="text-muted-foreground">{t("codexMcp.description")}</p>
			<div className="space-y-1 rounded-lg bg-muted/40 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
				<p>{t("codexMcp.secretHelp")}</p>
				<p>{t("codexMcp.newSessionsOnly")}</p>
			</div>
			<div className="flex flex-wrap items-center justify-between gap-2">
				<h3 className="font-medium">{t("codexMcp.configured")}</h3>
				<Button variant="outline" size="sm" disabled={busy} onClick={async () => {
					setError(""); setNotice("");
					if (await refresh()) setNotice(t("codexMcp.refreshed"));
				}}>{loading ? t("codexMcp.loading") : t("codexMcp.refresh")}</Button>
			</div>
			<p className="text-xs text-muted-foreground">{t("codexMcp.refreshHelp")}</p>
			{loading && <p role="status">{t("codexMcp.loading")}</p>}
			{listError && <p role="alert" className="text-destructive">{listError}</p>}
			{error && <p role="alert" className="text-destructive">{error}</p>}
			{notice && <p role="status" className="rounded-md bg-muted/40 p-3">{notice}</p>}
			{!loading && !listError && servers?.length === 0 && <div className="space-y-2 rounded-lg border border-dashed p-4">
				<h3 className="font-medium">{t("codexMcp.empty")}</h3>
				<p className="text-muted-foreground">{t("codexMcp.emptyHelp")}</p>
				<Button variant="outline" size="sm" disabled={pending} onClick={() => { reset(); nameInput.current?.focus(); }}>{t("codexMcp.createFirst")}</Button>
			</div>}
			{!!servers?.length && <ul className="divide-y rounded-lg border px-3">{servers.map(server => <li key={server.name} className="space-y-2 py-3">
				<div className="flex flex-wrap items-center justify-between gap-2">
					<div className="min-w-0"><span className="break-all font-medium">{server.name}</span><span className="ml-2 text-xs text-muted-foreground">{server.transport} · {server.enabled ? t("codexMcp.enabled") : t("codexMcp.disabled")}</span></div>
					<div className="flex flex-wrap gap-1">
						<Button variant="ghost" size="sm" disabled={busy || !!listError} aria-label={`${t("codexMcp.edit")} ${server.name}`} onClick={() => edit(server)}>{t("codexMcp.edit")}</Button>
						<Button variant="ghost" size="sm" disabled={busy || !!listError} aria-label={`${server.enabled ? t("codexMcp.disable") : t("codexMcp.enable")} ${server.name}`} onClick={() => void mutate(async () => {
							const result = await apiClient.PATCH("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: server.name } }, body: { enabled: !server.enabled } });
							if (result.error) throw result.error;
						}, t(server.enabled ? "codexMcp.disabledSuccess" : "codexMcp.enabledSuccess"))}>{server.enabled ? t("codexMcp.disable") : t("codexMcp.enable")}</Button>
						<Button variant="ghost" size="sm" disabled={busy || !!listError} aria-label={`${t("codexMcp.delete")} ${server.name}`} onClick={() => void mutate(async () => {
							const result = await apiClient.DELETE("/api/v1/settings/codex/mcp/{name}", { params: { path: { name: server.name } } });
							if (result.error) throw result.error;
							if (editing && draft.name === server.name) reset();
						}, t("codexMcp.deleted"))}>{t("codexMcp.delete")}</Button>
					</div>
				</div>
				{server.startupFailure && <p role="status" className="text-xs text-destructive">{t("codexMcp.startupFailed")}</p>}
				{server.missingReferences.length > 0 && <p role="status" className="break-words text-xs text-destructive">{t("codexMcp.missingReferences", { names: server.missingReferences.join(", ") })} {t("codexMcp.missingHelp")}</p>}
			</li>)}</ul>}
			<form className="space-y-3 rounded-lg border p-4" onSubmit={event => { event.preventDefault(); void save(); }}>
				<h3 className="font-medium">{editing ? t("codexMcp.editConfig") : t("codexMcp.addConfig")}</h3>
				{!editing && <div className="space-y-2">
					<p className="text-xs text-muted-foreground">{t("codexMcp.exampleHelp")}</p>
					<div className="flex flex-wrap gap-2">
						<Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => useExample("stdio")}>{t("codexMcp.stdioExample")}</Button>
						<Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => useExample("http")}>{t("codexMcp.httpExample")}</Button>
					</div>
				</div>}
				<div className="space-y-1">
					<label htmlFor={`${id}-name`}>{t("codexMcp.name")}</label>
					<Input id={`${id}-name`} ref={nameInput} required pattern="[a-zA-Z0-9_\-]{1,64}" disabled={editing || busy} placeholder={t("codexMcp.namePlaceholder")} aria-describedby={`${id}-name-help`} value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} />
					<p id={`${id}-name-help`} className="text-xs text-muted-foreground">{t("codexMcp.nameHelp")}</p>
				</div>
				<div className="space-y-1">
					<label htmlFor={`${id}-transport`}>{t("codexMcp.transport")}</label>
					<select id={`${id}-transport`} className="h-control-form w-full rounded-md bg-input/50 px-3 text-sm" disabled={busy} value={draft.transport} aria-describedby={`${id}-transport-help`} onChange={event => changeTransport(event.target.value as Transport)}>
						<option value="stdio">{t("codexMcp.stdio")}</option><option value="http">{t("codexMcp.http")}</option>
					</select>
					<p id={`${id}-transport-help`} className="text-xs text-muted-foreground">{t("codexMcp.transportHelp")}</p>
				</div>
				{draft.transport === "stdio" ? <>
					<div className="space-y-1">
						<label htmlFor={`${id}-command`}>{t("codexMcp.command")}</label>
						<Input id={`${id}-command`} required disabled={busy} placeholder={t("codexMcp.commandPlaceholder")} aria-describedby={`${id}-command-help`} value={draft.command ?? ""} onChange={event => setDraft({ ...draft, command: event.target.value })} />
						<p id={`${id}-command-help`} className="text-xs text-muted-foreground">{t("codexMcp.commandHelp")}</p>
					</div>
					<div className="space-y-1">
						<label htmlFor={`${id}-args`}>{t("codexMcp.args")}</label>
						<Input id={`${id}-args`} className="font-mono" disabled={busy} placeholder={JSON.stringify(stdioArgs)} aria-describedby={`${id}-args-help`} value={args} onChange={event => setArgs(event.target.value)} />
						<p id={`${id}-args-help`} className="text-xs text-muted-foreground">{t("codexMcp.argsHelp")}</p>
					</div>
				</> : <div className="space-y-1">
					<label htmlFor={`${id}-url`}>{t("codexMcp.url")}</label>
					<Input id={`${id}-url`} type="url" required disabled={busy} placeholder={httpUrl} aria-describedby={`${id}-url-help`} value={draft.url ?? ""} onChange={event => setDraft({ ...draft, url: event.target.value })} />
					<p id={`${id}-url-help`} className="text-xs text-muted-foreground">{t("codexMcp.urlHelp")}</p>
				</div>}
				<div className="space-y-1">
					<label htmlFor={`${id}-refs`}>{draft.transport === "stdio" ? t("codexMcp.envRefs") : t("codexMcp.headerRefs")}</label>
					<textarea id={`${id}-refs`} rows={3} className="w-full rounded-md bg-input/50 px-3 py-2 font-mono text-sm disabled:opacity-50" disabled={busy} spellCheck={false} placeholder={JSON.stringify(draft.transport === "stdio" ? { API_KEY: "MCP_API_KEY" } : httpRefs, null, 2)} aria-describedby={`${id}-refs-help`} value={refs} onChange={event => setRefs(event.target.value)} />
					<p id={`${id}-refs-help`} className="text-xs text-muted-foreground">{t("codexMcp.references")} {t(draft.transport === "stdio" ? "codexMcp.envHelp" : "codexMcp.headerHelp")}</p>
				</div>
				<label className="flex items-center gap-2"><input type="checkbox" disabled={busy} checked={draft.enabled} onChange={event => setDraft({ ...draft, enabled: event.target.checked })} />{t("codexMcp.enable")}</label>
				<div className="flex flex-wrap gap-2">
					<Button type="submit" disabled={busy}>{pending ? t("codexMcp.saving") : t("codexMcp.save")}</Button>
					{editing && <Button type="button" variant="outline" disabled={busy} onClick={reset}>{t("codexMcp.cancel")}</Button>}
				</div>
			</form>
		</div>
	</SettingsSection>;
}
