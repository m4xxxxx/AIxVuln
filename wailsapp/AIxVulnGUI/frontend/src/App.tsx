import { useEffect, useMemo, useRef, useState } from 'react';
import './App.css';
import { GetAPIBaseURL, GetBasicAuthPassword, GetBasicAuthUser } from '../wailsjs/go/main/App';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './components/ui/dialog';
import { Card, CardContent, CardHeader, CardTitle } from './components/ui/card';
import { Button } from './components/ui/button';
import { Badge } from './components/ui/badge';
import { ScrollArea } from './components/ui/scroll-area';
import { Input } from './components/ui/input';
import { Textarea } from './components/ui/textarea';
import ReactMarkdown from 'react-markdown';

type ApiResp<T> = {
    success?: boolean;
    result?: T;
    error?: any;
    Data?: T;
    data?: T;
};

function getRespData<T>(r: ApiResp<T>): T | undefined {
    return (r.result ?? r.Data ?? r.data) as any;
}

function App() {
    const [baseURL, setBaseURL] = useState<string>('');
    const [auth, setAuth] = useState<{ user: string; pass: string } | null>(null);
    const [initError, setInitError] = useState<string>('');
    const [token, setToken] = useState<string>(() => localStorage.getItem('aix_token') || '');
    const [isWailsMode, setIsWailsMode] = useState<boolean>(false);
    const [loginUser, setLoginUser] = useState<string>('');
    const [loginPass, setLoginPass] = useState<string>('');
    const [loginError, setLoginError] = useState<string>('');
    const [loginLoading, setLoginLoading] = useState<boolean>(false);
    const [projects, setProjects] = useState<string[]>([]);
    const [events, setEvents] = useState<any[]>([]);
    const [agentRuns, setAgentRuns] = useState<any[]>([]);
    const [digitalHumanRoster, setDigitalHumanRoster] = useState<Record<string, any[]>>({});
    const [exploitIdeas, setExploitIdeas] = useState<any[]>([]);
    const [exploitChains, setExploitChains] = useState<any[]>([]);
    const [containers, setContainers] = useState<any[]>([]);
    const [envInfo, setEnvInfo] = useState<any>(null);
    const [reports, setReports] = useState<Record<string, string>>({});
    const [projectStatus, setProjectStatus] = useState<string>('');
    const [projectIsRunning, setProjectIsRunning] = useState<boolean>(false);
    const [brainFinished, setBrainFinished] = useState<boolean>(false);
    const [view, setView] = useState<'home' | 'detail' | 'settings' | 'digital_humans' | 'report_templates'>('home');
    const [detailProject, setDetailProject] = useState<string>('');
    const [detailOpen, setDetailOpen] = useState<boolean>(false);
    const [detailTitle, setDetailTitle] = useState<string>('');
    const [detailKind, setDetailKind] = useState<'agent' | 'exploitIdea' | 'exploitChain' | 'container' | 'report' | 'json' | 'digitalHumanProfile'>('json');
    const [detailObj, setDetailObj] = useState<any>(null);
    const [projectName, setProjectName] = useState<string>('');
    const [taskContent, setTaskContent] = useState<string>('尽可能的挖掘项目中的漏洞，侧重挖掘未授权情况下能完成的高危攻击链');
    const [sourceType, setSourceType] = useState<'file' | 'git' | 'url'>('file');
    const [gitUrl, setGitUrl] = useState<string>('');
    const [fileUrl, setFileUrl] = useState<string>('');
    const fileRef = useRef<HTMLInputElement | null>(null);
    const wsRef = useRef<WebSocket | null>(null);
    const [chatMessages, setChatMessages] = useState<{ role: 'user' | 'system'; text: string; ts: string; persona_name?: string; avatar_file?: string }[]>([]);
    const [chatInput, setChatInput] = useState<string>('');
    const [chatSending, setChatSending] = useState<boolean>(false);
    const chatEndRef = useRef<HTMLDivElement | null>(null);
    const chatEndRefFull = useRef<HTMLDivElement | null>(null);
    const [chatFullscreen, setChatFullscreen] = useState<boolean>(false);
    const [toast, setToast] = useState<{ msg: string; type: 'ok' | 'err' } | null>(null);
    const [btnLoading, setBtnLoading] = useState<Record<string, boolean>>({});
    const [tokenUsage, setTokenUsage] = useState<{ prompt_tokens: number; completion_tokens: number; total_tokens: number; agents?: { label: string; prompt_tokens: number; completion_tokens: number; total_tokens: number }[] }>({ prompt_tokens: 0, completion_tokens: 0, total_tokens: 0, agents: [] });
    const [showTokenPanel, setShowTokenPanel] = useState<boolean>(false);
    const [contextBreakdown, setContextBreakdown] = useState<{ sections: { name: string; tokens: number }[]; total: number; max_context: number }>({ sections: [], total: 0, max_context: 0 });
    const [configData, setConfigData] = useState<Record<string, Record<string, string>>>({});
    const [configDraft, setConfigDraft] = useState<Record<string, Record<string, string>>>({});
    const [configLoading, setConfigLoading] = useState<boolean>(false);
    const [configSaving, setConfigSaving] = useState<boolean>(false);
    const [newSectionName, setNewSectionName] = useState<string>('');
    const [newKeyInputs, setNewKeyInputs] = useState<Record<string, { key: string; value: string }>>({});
    const [modelOptions, setModelOptions] = useState<Record<string, string[]>>({});
    const [modelLoading, setModelLoading] = useState<Record<string, boolean>>({});
    const [digitalHumans, setDigitalHumans] = useState<any[]>([]);
    const [dhLoading, setDhLoading] = useState<boolean>(false);
    const [dhEditing, setDhEditing] = useState<any | null>(null);
    const [reportTemplates, setReportTemplates] = useState<Record<string, string>>({});
    const [reportTemplatesDraft, setReportTemplatesDraft] = useState<Record<string, string>>({});
    const [rtLoading, setRtLoading] = useState<boolean>(false);
    const [rtSaving, setRtSaving] = useState<boolean>(false);
    const [initChecked, setInitChecked] = useState<boolean>(false);
    const [needsInit, setNeedsInit] = useState<boolean>(false);
    const [dockerImages, setDockerImages] = useState<Record<string, boolean>>({});
    const [initStep, setInitStep] = useState<'user' | 'docker' | 'done'>('user');
    const [initUsername, setInitUsername] = useState<string>('');
    const [initPassword, setInitPassword] = useState<string>('');
    const [initPassword2, setInitPassword2] = useState<string>('');
    const [setupError, setSetupError] = useState<string>('');
    const [initLoading, setInitLoading] = useState<boolean>(false);
    const [dockerBuilding, setDockerBuilding] = useState<Record<string, boolean>>({});
    const [dockerBuildOutput, setDockerBuildOutput] = useState<Record<string, string>>({});
    const [customRegistry, setCustomRegistry] = useState<string>('');

    function formatTokens(n: number): string {
        if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M';
        if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
        return String(n);
    }

    function showToast(msg: string, type: 'ok' | 'err' = 'ok') {
        setToast({ msg, type });
        setTimeout(() => setToast(null), 2500);
    }

    const authHeader = useMemo(() => {
        // Wails mode: use BasicAuth from Go bridge (only if credentials are non-empty)
        if (isWailsMode && auth && auth.user && auth.pass) {
            return 'Basic ' + btoa(`${auth.user}:${auth.pass}`);
        }
        // Token-based auth (web mode or Wails mode with empty credentials)
        if (token) return 'Bearer ' + token;
        return '';
    }, [auth, token, isWailsMode]);

    function handleLogout() {
        setToken('');
        localStorage.removeItem('aix_token');
    }

    async function handleLogin() {
        if (!loginUser.trim() || !loginPass.trim()) return;
        setLoginLoading(true);
        setLoginError('');
        try {
            let loginBase = baseURL;
            if (!loginBase) {
                const origin = (typeof window !== 'undefined' && window.location?.origin) || '';
                if (origin && origin !== 'null') {
                    const u = new URL(origin);
                    const host = String(u.hostname || '').toLowerCase();
                    const port = String(u.port || '');
                    if ((host === 'localhost' || host === '127.0.0.1') && port && port !== '9999') {
                        loginBase = 'http://127.0.0.1:9999';
                    }
                }
            }
            const resp = await fetch(`${loginBase}/login`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username: loginUser.trim(), password: loginPass.trim() }),
            });
            const js = await resp.json();
            if (!resp.ok || !js?.success) {
                setLoginError(js?.error || '登录失败');
                return;
            }
            const t = js.token as string;
            setToken(t);
            localStorage.setItem('aix_token', t);
            setLoginError('');
        } catch (e: any) {
            setLoginError(e?.message || '网络错误');
        } finally {
            setLoginLoading(false);
        }
    }

    const detailLiveRef = useRef<{ open: boolean; kind: string; obj: any }>({ open: false, kind: 'json', obj: null });

    useEffect(() => {
        detailLiveRef.current = { open: detailOpen, kind: detailKind, obj: detailObj };
    }, [detailOpen, detailKind, detailObj]);

    function cleanupDetailState() {
        wsRef.current?.close();
        setDetailProject('');
        setAgentRuns([]);
        setExploitIdeas([]);
        setExploitChains([]);
        setContainers([]);
        setEnvInfo(null);
        setReports({});
        setProjectStatus('');
        setProjectIsRunning(false);
        setBrainFinished(false);
        setEvents([]);
    }

    function applyRouteFromHash() {
        const h = String(window.location?.hash ?? '');
        const raw = h.startsWith('#') ? h.slice(1) : h;
        const p = raw.startsWith('/') ? raw : '/';

        // Routes:
        // - #/                 => home
        // - #/project/:name    => detail
        if (p === '/' || p === '') {
            cleanupDetailState();
            setView('home');
            return;
        }

        if (p === '/settings') {
            cleanupDetailState();
            setView('settings');
            return;
        }

        if (p === '/digital_humans') {
            cleanupDetailState();
            setView('digital_humans');
            return;
        }

        if (p === '/report_templates') {
            cleanupDetailState();
            setView('report_templates');
            return;
        }

        const m = p.match(/^\/project\/(.+)$/);
        if (m) {
            const name = decodeURIComponent(m[1]);
            setDetailProject(name);
            setView('detail');
            return;
        }

        // Unknown hash => home
        cleanupDetailState();
        setView('home');
    }

    useEffect(() => {
        // Hash routing: always load at '/', use hash to switch views.
        applyRouteFromHash();
        const onHash = () => applyRouteFromHash();
        window.addEventListener('hashchange', onHash);
        return () => window.removeEventListener('hashchange', onHash);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Auto-fetch data when view changes and auth is ready.
    useEffect(() => {
        if (!authHeader || !initChecked) return;
        if (view === 'settings') fetchConfig();
        if (view === 'digital_humans') fetchDigitalHumans();
        if (view === 'report_templates') fetchReportTemplates();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [view, authHeader, initChecked]);

    useEffect(() => {
        (async () => {
            let resolvedBase = '';
            try {
                const url = await GetAPIBaseURL();
                const user = await GetBasicAuthUser();
                const pass = await GetBasicAuthPassword();
                setBaseURL(url);
                resolvedBase = url;
                setAuth({ user, pass });
                setIsWailsMode(true);
                setInitError('');
            } catch (e: any) {
                // WEB mode (served by gin) or browser dev mode without Wails bridge.
                setIsWailsMode(false);
                const origin = (typeof window !== 'undefined' && window.location && window.location.origin) ? window.location.origin : '';
                let useLocalDevAPI = false;
                try {
                    if (origin && origin !== 'null') {
                        const u = new URL(origin);
                        const host = String(u.hostname || '').toLowerCase();
                        const port = String(u.port || '');
                        if ((host === 'localhost' || host === '127.0.0.1') && port && port !== '9999') {
                            useLocalDevAPI = true;
                        }
                    }
                } catch {
                    // ignore
                }
                if (useLocalDevAPI) {
                    resolvedBase = 'http://127.0.0.1:9999';
                    setBaseURL(resolvedBase);
                } else {
                    setBaseURL('');
                }
                setInitError(String(e?.message ?? e ?? 'init failed'));
            }
            // Check init status (first-run detection).
            try {
                const initBase = resolvedBase || '';
                const resp = await fetch(`${initBase}/init_status`);
                const js = await resp.json();
                const dimgs = js.docker_images || {};
                setDockerImages(dimgs);
                if (js.needs_user) {
                    setNeedsInit(true);
                    setInitStep('user');
                } else if (!dimgs['aisandbox'] || !dimgs['java_env']) {
                    setNeedsInit(true);
                    setInitStep('docker');
                }
            } catch {
                // Server not reachable yet, skip init check.
            }
            // Validate stored token: tokenSecret regenerates on server restart,
            // so a stale localStorage token must be cleared to show the login page.
            const storedToken = localStorage.getItem('aix_token');
            if (storedToken) {
                try {
                    const hResp = await fetch(`${resolvedBase || ''}/healthz`, {
                        headers: { Authorization: 'Bearer ' + storedToken },
                    });
                    if (hResp.status === 401) {
                        localStorage.removeItem('aix_token');
                        setToken('');
                    }
                } catch {
                    // Server not reachable, keep token for now.
                }
            }
            setInitChecked(true);
        })();
    }, []);

    function badgeVariantForRunState(s: any): any {
        const v = String(s ?? '').toLowerCase();
        if (!v) return 'default';
        if (v.includes('running')) return 'warning';
        if (v.includes('done') || v.includes('completed') || v.includes('success')) return 'success';
        if (v.includes('fail') || v.includes('error')) return 'destructive';
        return 'secondary';
    }

    function badgeVariantForExploitState(s: any): any {
        const v = String(s ?? '');
        if (!v) return 'default';
        if (v.includes('可利用')) return 'success';
        if (v.includes('正在验证')) return 'warning';
        if (v.includes('验证失败') || v.includes('审核失败') || v.toLowerCase().includes('fail')) return 'destructive';
        return 'secondary';
    }

    function shortText(x: any, max = 240): string {
        const s = String(x ?? '');
        if (s.length <= max) return s;
        return s.slice(0, max) + '…';
    }

    function isTruncated(x: any, max = 240): boolean {
        const s = String(x ?? '');
        return s.length > max;
    }

    async function apiPost<T>(path: string, body: any): Promise<T | undefined> {
        let useBase = baseURL;
        if (!useBase) {
            try {
                const u = await GetAPIBaseURL();
                if (u) { useBase = u; setBaseURL(u); }
            } catch { /* ignore */ }
        }
        const url = `${useBase}${path}`;
        const resp = await fetch(url, {
            method: 'POST',
            headers: { Authorization: authHeader, 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (resp.status === 401) { handleLogout(); throw new Error('登录已过期，请重新登录'); }
        if (!resp.ok) throw new Error(`apiPost failed: ${resp.status}`);
        const js = (await resp.json()) as ApiResp<T>;
        return getRespData(js);
    }

    async function apiGet<T>(path: string): Promise<T | undefined> {
        // In `wails dev`, there can be a short init race where baseURL is still empty.
        // If we have the Wails bridge, try to self-heal by fetching the API baseURL.
        let useBase = baseURL;
        if (!useBase) {
            try {
                const u = await GetAPIBaseURL();
                if (u) {
                    useBase = u;
                    setBaseURL(u);
                }
            } catch {
                // ignore
            }
        }

        const url = `${useBase}${path}`;
        let resp: Response;
        try {
            resp = await fetch(url, {
                headers: {
                    Authorization: authHeader,
                },
            });
        } catch (e: any) {
            console.error('apiGet network error', { url, error: e });
            throw e;
        }

        if (resp.status === 401) { handleLogout(); throw new Error('登录已过期，请重新登录'); }
        if (!resp.ok) {
            let text = '';
            try {
                text = await resp.text();
            } catch {
                // ignore
            }
            console.error('apiGet non-2xx', { url, status: resp.status, body: text });
            throw new Error(`apiGet failed: ${resp.status}`);
        }

        const js = (await resp.json()) as ApiResp<T>;
        return getRespData(js);
    }

    async function refreshProjects() {
        if (!authHeader) return;

        const data = await apiGet<any>('/projects');
        if (Array.isArray(data)) {
            setProjects(data);
        }
    }

    useEffect(() => {
        refreshProjects();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [authHeader]);

    async function createProject() {
        if (!authHeader) return;
        if (sourceType === 'file' && !fileRef.current?.files?.[0]) { showToast('请选择源码压缩包', 'err'); return; }
        if (sourceType === 'git' && !gitUrl.trim()) { showToast('请输入 Git 仓库地址', 'err'); return; }
        if (sourceType === 'url' && !fileUrl.trim()) { showToast('请输入压缩包下载链接', 'err'); return; }
        setBtnLoading(prev => ({ ...prev, create: true }));
        try {
            const fd = new FormData();
            fd.append('source_type', sourceType);
            fd.append('taskContent', taskContent);
            if (sourceType === 'file') {
                fd.append('file', fileRef.current!.files![0]);
            } else if (sourceType === 'git') {
                fd.append('git_url', gitUrl.trim());
            } else if (sourceType === 'url') {
                fd.append('file_url', fileUrl.trim());
            }
            const name = projectName.trim();
            const q = name ? `?projectName=${encodeURIComponent(name)}` : '';
            const resp = await fetch(`${baseURL}/projects/create${q}`, {
                method: 'POST',
                headers: { Authorization: authHeader },
                body: fd,
            });
            const js = await resp.json();
            if (!resp.ok || js?.error) {
                showToast(js?.error || '创建失败', 'err');
                return;
            }
            await refreshProjects();
            showToast('项目创建成功');
        } catch (e: any) {
            showToast(e?.message || '创建失败', 'err');
        } finally {
            setBtnLoading(prev => ({ ...prev, create: false }));
        }
    }

    async function enterProject(name: string) {
        window.location.hash = `#/project/${encodeURIComponent(name)}`;
    }

    function leaveProject() {
        window.location.hash = '#/';
    }

    async function fetchConfig() {
        setConfigLoading(true);
        try {
            const resp = await fetch(`${baseURL}/config`, { headers: { Authorization: authHeader } });
            const js = await resp.json();
            const d = getRespData<Record<string, Record<string, string>>>(js) ?? {};
            setConfigData(JSON.parse(JSON.stringify(d)));
            setConfigDraft(JSON.parse(JSON.stringify(d)));
        } catch (e: any) {
            showToast('加载配置失败: ' + (e?.message || ''), 'err');
        } finally {
            setConfigLoading(false);
        }
    }

    async function saveConfig() {
        setConfigSaving(true);
        try {
            const resp = await fetch(`${baseURL}/config`, {
                method: 'PUT',
                headers: { Authorization: authHeader, 'Content-Type': 'application/json' },
                body: JSON.stringify(configDraft),
            });
            const js = await resp.json();
            if (!resp.ok || js?.error) {
                showToast('保存失败: ' + (js?.error || ''), 'err');
                return;
            }
            showToast(getRespData<string>(js) || '保存成功');
            setConfigData(JSON.parse(JSON.stringify(configDraft)));
        } catch (e: any) {
            showToast('保存失败: ' + (e?.message || ''), 'err');
        } finally {
            setConfigSaving(false);
        }
    }

    function updateConfigDraft(section: string, key: string, value: string) {
        setConfigDraft(prev => ({
            ...prev,
            [section]: { ...(prev[section] || {}), [key]: value },
        }));
    }

    function deleteConfigKey(section: string, key: string) {
        setConfigDraft(prev => {
            const sec = { ...(prev[section] || {}) };
            delete sec[key];
            if (Object.keys(sec).length === 0) {
                const next = { ...prev };
                delete next[section];
                return next;
            }
            return { ...prev, [section]: sec };
        });
    }

    function deleteConfigSection(section: string) {
        setConfigDraft(prev => {
            const next = { ...prev };
            delete next[section];
            return next;
        });
    }

    function addConfigSection(name: string) {
        if (!name.trim()) return;
        setConfigDraft(prev => ({ ...prev, [name.trim()]: { ...(prev[name.trim()] || {}) } }));
        setNewSectionName('');
    }

    function addConfigKey(section: string) {
        const input = newKeyInputs[section];
        if (!input?.key?.trim()) return;
        updateConfigDraft(section, input.key.trim(), input.value || '');
        setNewKeyInputs(prev => ({ ...prev, [section]: { key: '', value: '' } }));
    }

    async function fetchModels(section: string) {
        const draft = configDraft[section];
        if (!draft) return;
        const baseUrl = (draft['BASE_URL'] || '').trim();
        const apiKey = (draft['OPENAI_API_KEY'] || '').trim();
        if (!baseUrl || !apiKey) return;
        setModelLoading(prev => ({ ...prev, [section]: true }));
        try {
            const resp = await fetch(`${baseURL}/models?base_url=${encodeURIComponent(baseUrl)}&api_key=${encodeURIComponent(apiKey)}`, {
                headers: { Authorization: authHeader },
            });
            const js = await resp.json();
            const list = getRespData<string[]>(js) ?? [];
            if (list.length > 0) {
                setModelOptions(prev => ({ ...prev, [section]: list.sort() }));
            } else {
                setModelOptions(prev => ({ ...prev, [section]: [] }));
            }
        } catch {
            setModelOptions(prev => ({ ...prev, [section]: [] }));
        } finally {
            setModelLoading(prev => ({ ...prev, [section]: false }));
        }
    }

    async function fetchDigitalHumans() {
        setDhLoading(true);
        try {
            const resp = await fetch(`${baseURL}/digital_humans`, { headers: { Authorization: authHeader } });
            const js = await resp.json();
            setDigitalHumans(getRespData<any[]>(js) ?? []);
        } catch (e: any) {
            showToast('加载数字人失败: ' + (e?.message || ''), 'err');
        } finally {
            setDhLoading(false);
        }
    }

    async function saveDigitalHumanAPI(dh: any) {
        try {
            const resp = await fetch(`${baseURL}/digital_humans`, {
                method: 'POST',
                headers: { Authorization: authHeader, 'Content-Type': 'application/json' },
                body: JSON.stringify(dh),
            });
            const js = await resp.json();
            if (!resp.ok || js?.error) {
                showToast('保存失败: ' + (js?.error || ''), 'err');
                return;
            }
            showToast(getRespData<string>(js) || '保存成功');
            fetchDigitalHumans();
            setDhEditing(null);
        } catch (e: any) {
            showToast('保存失败: ' + (e?.message || ''), 'err');
        }
    }

    async function deleteDigitalHumanAPI(id: string) {
        try {
            const resp = await fetch(`${baseURL}/digital_humans/${encodeURIComponent(id)}`, {
                method: 'DELETE',
                headers: { Authorization: authHeader },
            });
            const js = await resp.json();
            if (!resp.ok || js?.error) {
                showToast('删除失败: ' + (js?.error || ''), 'err');
                return;
            }
            showToast(getRespData<string>(js) || '删除成功');
            fetchDigitalHumans();
        } catch (e: any) {
            showToast('删除失败: ' + (e?.message || ''), 'err');
        }
    }

    async function fetchReportTemplates() {
        setRtLoading(true);
        try {
            const resp = await fetch(`${baseURL}/report_templates`, { headers: { Authorization: authHeader } });
            const js = await resp.json();
            const data = getRespData<Record<string, string>>(js) ?? {};
            setReportTemplates(data);
            setReportTemplatesDraft(data);
        } catch (e: any) {
            showToast('加载报告模板失败: ' + (e?.message || ''), 'err');
        } finally {
            setRtLoading(false);
        }
    }

    async function saveReportTemplates() {
        setRtSaving(true);
        try {
            const resp = await fetch(`${baseURL}/report_templates`, {
                method: 'PUT',
                headers: { Authorization: authHeader, 'Content-Type': 'application/json' },
                body: JSON.stringify(reportTemplatesDraft),
            });
            const js = await resp.json();
            if (!resp.ok || js?.error) {
                showToast(js?.error || '保存失败', 'err');
                return;
            }
            showToast(getRespData<string>(js) || '保存成功');
            setReportTemplates({ ...reportTemplatesDraft });
        } catch (e: any) {
            showToast('保存失败: ' + (e?.message || ''), 'err');
        } finally {
            setRtSaving(false);
        }
    }

    function openDetail(kind: 'agent' | 'exploitIdea' | 'exploitChain' | 'container' | 'report' | 'json' | 'digitalHumanProfile', title: string, obj: any) {
        setDetailTitle(title);
        setDetailKind(kind);
        setDetailObj(obj);
        setDetailOpen(true);
    }

    function openReportPreview(id: string, filename: string, content: string) {
        setDetailTitle(filename || id);
        setDetailKind('report');
        setDetailObj({ id, filename, content });
        setDetailOpen(true);
    }

    function closeDetail() {
        setDetailOpen(false);
    }

    async function refreshProjectDetail() {
        if (!detailProject) return;
        const name = encodeURIComponent(detailProject);
        const proj = await apiGet<any>(`/projects/${name}`);

        const runs = proj?.agentRuns;
        if (Array.isArray(runs)) setAgentRuns(runs);

        const dh = proj?.digitalHumans;
        if (dh && typeof dh === 'object') setDigitalHumanRoster(dh as any);

        const ideas = proj?.exploitIdeas;
        if (Array.isArray(ideas)) setExploitIdeas(ideas);

        const chains = proj?.exploitChains;
        if (Array.isArray(chains)) setExploitChains(chains);

        const cs = proj?.containers;
        if (Array.isArray(cs)) setContainers(cs);

        const es = proj?.envInfo;
        if (es) setEnvInfo(es);

        const rs = proj?.reports;
        if (rs && typeof rs === 'object' && !Array.isArray(rs)) setReports(rs);

        setProjectStatus(String(proj?.status ?? ''));
        setProjectIsRunning(Boolean(proj?.isRunning));
        setBrainFinished(Boolean(proj?.brainFinished));

        // Load token usage.
        try {
            const tu = await apiGet<any>(`/projects/${name}/token_usage`);
            if (tu && typeof tu === 'object') {
                setTokenUsage({
                    prompt_tokens: Number(tu?.prompt_tokens ?? 0),
                    completion_tokens: Number(tu?.completion_tokens ?? 0),
                    total_tokens: Number(tu?.total_tokens ?? 0),
                    agents: Array.isArray(tu?.agents) ? tu.agents : [],
                });
            }
        } catch { /* ignore */ }

        // Load context breakdown.
        try {
            const cb = await apiGet<any>(`/projects/${name}/context_breakdown`);
            if (cb && typeof cb === 'object') {
                setContextBreakdown({
                    sections: Array.isArray(cb?.sections) ? cb.sections : [],
                    total: Number(cb?.total ?? 0),
                    max_context: Number(cb?.max_context ?? 0),
                });
            }
        } catch { /* ignore */ }

        // Load persisted chat messages.
        try {
            const chatMsgs = await apiGet<any[]>(`/projects/${name}/chat/messages`);
            if (Array.isArray(chatMsgs) && chatMsgs.length > 0) {
                setChatMessages(chatMsgs.map((m: any) => ({
                    role: (m?.role === 'user' ? 'user' : 'system') as 'user' | 'system',
                    text: String(m?.text ?? ''),
                    ts: String(m?.ts ?? ''),
                    persona_name: m?.persona_name || undefined,
                    avatar_file: m?.avatar_file || undefined,
                })));
            }
        } catch { /* ignore */ }
    }

    async function fetchReportText(project: string, id: string): Promise<string> {
        const name = encodeURIComponent(project);
        const rid = encodeURIComponent(id);
        const resp = await fetch(`${baseURL}/projects/${name}/reports/download/${rid}`, {
            headers: {
                Authorization: authHeader,
            },
        });
        if (!resp.ok) {
            throw new Error(`fetch report failed: ${resp.status}`);
        }
        return await resp.text();
    }

    async function downloadFileWithAuth(url: string, filename: string) {
        const resp = await fetch(url, {
            headers: {
                Authorization: authHeader,
            },
        });
        if (!resp.ok) {
            throw new Error(`download failed: ${resp.status}`);
        }
        const blob = await resp.blob();
        const objUrl = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = objUrl;
        a.download = filename || 'download';
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(objUrl);
    }

    const rowStyle: any = {
        display: 'grid',
        gridTemplateColumns: '160px 1fr',
        gap: 12,
        padding: '10px 0',
        borderBottom: '1px solid rgba(255,255,255,0.08)',
        minWidth: 0,
    };
    const labelStyle: any = { opacity: 0.75, fontWeight: 700 };
    const valStyle: any = { opacity: 0.95, wordBreak: 'break-word', overflow: 'hidden', minWidth: 0 };

    function Section({ title, children }: { title: string; children: any }) {
        return (
            <div style={{ marginTop: 14 }}>
                <div style={{ fontWeight: 800, letterSpacing: 0.2, marginBottom: 8 }}>{title}</div>
                <div>{children}</div>
            </div>
        );
    }

    function FieldRow({ label, value }: { label: string; value: any }) {
        if (value === undefined || value === null || value === '') return null;
        const isLong = typeof value === 'string' && value.length > 180;
        return (
            <div style={rowStyle}>
                <div style={labelStyle}>{label}</div>
                <div style={valStyle}>
                    {isLong ? (
                        <pre className="aix-pre" style={{ maxHeight: 260, overflow: 'auto', margin: 0 }}>
                            {String(value)}
                        </pre>
                    ) : (
                        <div>{typeof value === 'string' ? value : JSON.stringify(value)}</div>
                    )}
                </div>
            </div>
        );
    }

    function get(obj: any, keys: string[]) {
        for (const k of keys) {
            const v = obj?.[k];
            if (v !== undefined) return v;
        }
        return undefined;
    }

    function renderDetailContent() {
        const kind = detailKind;
        const obj = detailObj;

        if (!obj) return <div className="aix-empty">无</div>;

        if (kind === 'agent') {
            return (
                <div style={{ paddingTop: 6 }}>
                    <Section title="基础信息">
                        <FieldRow label="运行状态" value={get(obj, ['RunState', 'runState'])} />
                    </Section>
                    <Section title="RUNTask">
                        <FieldRow label="RUNTask" value={get(obj, ['RUNTask', 'runTask'])} />
                    </Section>
                    <Section title="RUNSummary">
                        <FieldRow label="RUNSummary" value={get(obj, ['RUNSummary', 'runSummary'])} />
                    </Section>
                </div>
            );
        }

        if (kind === 'report') {
            const content = String(obj?.content ?? '');
            return (
                <div className="space-y-3">
                    <div className="flex items-center justify-between gap-2">
                        <div className="text-xs text-muted-foreground">{String(obj?.id ?? '')}</div>
                        <Button size="sm" variant="secondary" onClick={() => {
                            const name = encodeURIComponent(detailProject);
                            const rid = encodeURIComponent(String(obj?.id ?? ''));
                            const fn = String(obj?.filename ?? 'report.md');
                            downloadFileWithAuth(`${baseURL}/projects/${name}/reports/download/${rid}`, fn);
                        }}>下载</Button>
                    </div>
                    <div className="rounded-lg border border-border bg-background/20 p-3">
                        <div className="prose prose-invert max-w-none text-sm">
                            <ReactMarkdown>{content}</ReactMarkdown>
                        </div>
                    </div>
                </div>
            );
        }

        if (detailKind === 'container') {
            return (
                <div style={{ paddingTop: 6 }}>
                    <Section title="基础信息">
                        <FieldRow label="containerId" value={get(obj, ['containerId', 'ContainerId'])} />
                        <FieldRow label="image" value={get(obj, ['image', 'Image'])} />
                        <FieldRow label="containerIP" value={get(obj, ['containerIP', 'ContainerIP'])} />
                        <FieldRow label="webPort" value={(() => { const wp = obj?.webPort ?? obj?.WebPort; return Array.isArray(wp) && wp.length > 0 ? wp.join(', ') : '-'; })()} />
                        <FieldRow label="type" value={get(obj, ['type', 'Type'])} />
                    </Section>
                </div>
            );
        }

        if (detailKind === 'exploitIdea') {
            const ep = get(obj, ['exploit_point', 'ExploitPoint']);
            return (
                <div style={{ paddingTop: 6 }}>
                    <Section title="基本信息">
                        <FieldRow label="ExploitIdeaId" value={get(obj, ['exploitIdeaId', 'ExploitIdeaId'])} />
                        <FieldRow label="状态" value={get(obj, ['state', 'State'])} />
                        <FieldRow label="危害" value={get(obj, ['harm', 'Harm'])} />
                        <FieldRow label="扩大化思路" value={get(obj, ['extend_idea', 'ExtendIdea'])} />
                        <FieldRow label="利用条件" value={get(obj, ['condition', 'Condition'])} />
                    </Section>

                    <Section title="ExploitPoint">
                        <FieldRow label="标题" value={get(ep, ['title', 'Title'])} />
                        <FieldRow label="类型" value={get(ep, ['type', 'Type'])} />
                        <FieldRow label="路由/端点" value={get(ep, ['route_or_endpoint', 'RouteOrEndpoint'])} />
                        <FieldRow label="文件" value={get(ep, ['file', 'File'])} />
                        <FieldRow label="函数/方法" value={get(ep, ['function_or_method', 'FunctionOrMethod'])} />
                        <FieldRow label="参数" value={get(ep, ['params', 'Params'])} />
                        <FieldRow label="Payload 思路" value={get(ep, ['payload_idea', 'PayloadIdea'])} />
                        <FieldRow label="预期影响" value={get(ep, ['expected_impact', 'ExpectedImpact'])} />
                        <FieldRow label="置信度" value={get(ep, ['confidence', 'Confidence'])} />
                        <FieldRow label="ExploitId" value={get(ep, ['exploit_id', 'ExploitId'])} />
                    </Section>

                    <Section title="证据与 PoC">
                        <FieldRow label="Evidence" value={get(obj, ['evidence', 'Evidence'])} />
                        <FieldRow label="PoC" value={get(obj, ['poc', 'Poc'])} />
                    </Section>
                </div>
            );
        }

        if (detailKind === 'digitalHumanProfile') {
            const avatarSrc = String(obj?.avatar_src ?? '');
            const personaName = String(obj?.persona_name ?? '-');
            const gender = String(obj?.gender ?? '-');
            const personality = String(obj?.personality ?? '-');
            const age = Number(obj?.age ?? 0);
            const role = String(obj?.role ?? '-');
            const dhId = String(obj?.digital_human_id ?? '-');
            const state = String(obj?.state ?? '-');
            const agentId = String(obj?.agent_id ?? '');
            const runState = obj?.RunState;
            const runTask = obj?.RUNTask;
            const runSummary = obj?.RUNSummary;
            const lastSummary = String(obj?.last_summary ?? '');
            const lastTask = String(obj?.last_task ?? '');
            const summaryToShow = runSummary ? String(runSummary) : lastSummary;
            const taskToShow = runTask ? String(runTask) : lastTask;
            return (
                <div style={{ paddingTop: 6 }}>
                    <div className="flex items-center gap-4 mb-4">
                        {avatarSrc && (
                            <img
                                src={avatarSrc}
                                alt={personaName}
                                className="h-16 w-16 rounded-full border-2 border-border object-cover"
                            />
                        )}
                        <div>
                            <div className="text-lg font-bold">{personaName}</div>
                            <div className="text-sm text-muted-foreground">{role}</div>
                        </div>
                    </div>
                    <Section title="基本信息">
                        <FieldRow label="姓名" value={personaName} />
                        <FieldRow label="职业" value={role} />
                        <FieldRow label="性格" value={personality} />
                        <FieldRow label="数字人ID" value={dhId} />
                    </Section>
                    <Section title="状态">
                        <FieldRow label="当前状态" value={state === 'busy' ? '忙碌' : '空闲'} />
                        {runState ? <FieldRow label="运行状态" value={String(runState)} /> : null}
                    </Section>
                    {taskToShow ? (
                        <Section title={runTask ? '当前任务' : '上次任务'}>
                            <div className="text-sm text-foreground whitespace-pre-wrap break-words">{taskToShow}</div>
                        </Section>
                    ) : null}
                    {summaryToShow ? (
                        <Section title={runSummary ? '执行摘要' : '上次任务总结'}>
                            <div className="prose prose-invert prose-sm max-w-none break-words text-sm">
                                <ReactMarkdown>{summaryToShow}</ReactMarkdown>
                            </div>
                        </Section>
                    ) : null}
                </div>
            );
        }

        if (detailKind === 'exploitChain') {
            const eideas = get(obj, ['exploit_idea', 'ExploitIdea']);
            return (
                <div style={{ paddingTop: 6 }}>
                    <Section title="基本信息">
                        <FieldRow label="ExploitChainId" value={get(obj, ['exploit_chain_id', 'ExploitChainId'])} />
                        <FieldRow label="状态" value={get(obj, ['state', 'State'])} />
                        <FieldRow label="组链思路" value={get(obj, ['idea', 'Idea'])} />
                    </Section>

                    <Section title="链路包含的 ExploitIdea">
                        {Array.isArray(eideas) && eideas.length > 0 ? (
                            <div style={{ display: 'grid', gap: 10 }}>
                                {eideas.map((one: any, idx: number) => (
                                    <div key={one?.exploitIdeaId ?? one?.ExploitIdeaId ?? idx} className="aix-item">
                                        <div style={{ fontWeight: 800 }}>{String(one?.exploitIdeaId ?? one?.ExploitIdeaId ?? 'exploitIdea')}</div>
                                        <div style={{ opacity: 0.8, marginTop: 4 }}>状态：{String(one?.state ?? one?.State ?? '-')}</div>
                                    </div>
                                ))}
                            </div>
                        ) : (
                            <div className="aix-empty">无</div>
                        )}
                    </Section>

                    <Section title="证据与 PoC">
                        <FieldRow label="Evidence" value={get(obj, ['evidence', 'Evidence'])} />
                        <FieldRow label="PoC" value={get(obj, ['poc', 'Poc'])} />
                    </Section>
                </div>
            );
        }

        return (
            <pre className="aix-pre" style={{ maxHeight: '70vh', overflow: 'auto' }}>
                {JSON.stringify(obj, null, 2)}
            </pre>
        );
    }

    async function startProject(name: string) {
        const key = `start_${name}`;
        setBtnLoading(prev => ({ ...prev, [key]: true }));
        try {
            await apiGet(`/projects/${encodeURIComponent(name)}/start`);
            showToast(`${name} 已启动`);
        } catch (e: any) {
            showToast(e?.message || '启动失败', 'err');
        } finally {
            setBtnLoading(prev => ({ ...prev, [key]: false }));
        }
    }
    async function stopProject(name: string) {
        const key = `stop_${name}`;
        setBtnLoading(prev => ({ ...prev, [key]: true }));
        try {
            await apiGet(`/projects/${encodeURIComponent(name)}/cancel`);
            showToast(`${name} 已停止`);
        } catch (e: any) {
            showToast(e?.message || '停止失败', 'err');
        } finally {
            setBtnLoading(prev => ({ ...prev, [key]: false }));
        }
    }
    async function deleteProject(name: string) {
        const key = `del_${name}`;
        setBtnLoading(prev => ({ ...prev, [key]: true }));
        try {
            await apiGet(`/projects/${encodeURIComponent(name)}/del`);
            await refreshProjects();
            showToast(`${name} 已删除`);
        } catch (e: any) {
            showToast(e?.message || '删除失败', 'err');
        } finally {
            setBtnLoading(prev => ({ ...prev, [key]: false }));
        }
    }

    function connectWS(projectName: string) {
        if (!authHeader) return;
        wsRef.current?.close();
        const name = encodeURIComponent(projectName);
        let wsBase = '';
        // If baseURL is absolute (dev / GUI), connect websocket to that host.
        if (baseURL && /^https?:\/\//.test(baseURL)) {
            wsBase = baseURL;
        } else if (typeof window !== 'undefined' && window.location.origin) {
            // Same-origin (web mode)
            wsBase = window.location.origin;
        }
        if (!wsBase) return;
        const wsToken = token || '';
        const wsURL = wsBase.replace(/^http/, 'ws') + `/ws?projectName=${name}&token=${encodeURIComponent(wsToken)}`;
        const ws = new WebSocket(wsURL);
        ws.onopen = () => {
            // Connected
        };
        ws.onmessage = (ev) => {
            try {
                const msg = JSON.parse(ev.data);

                if (msg?.type === 'string' && typeof msg?.data === 'string') {
                    // legacy event stream: no longer displayed
                    return;
                }
                if (msg?.type === 'ReportAdd' && msg?.data && typeof msg.data === 'object') {
                    setReports((prev) => ({ ...prev, ...(msg.data as any) }));
                    return;
                }
                if (msg?.type === 'EnvInfo' && msg?.data) {
                    const d: any = msg.data;
                    setEnvInfo(d?.env ?? d);
                    return;
                }

                if (msg?.type === 'ContainerAdd' && msg?.data) {
                    setContainers((prev) => {
                        const c = msg.data as any;
                        const id = c?.containerId ?? c?.ContainerId;
                        if (!id) return prev;
                        const exists = prev.some((x: any) => (x?.containerId ?? x?.ContainerId) === id);
                        if (exists) return prev;
                        return [c, ...prev];
                    });
                    return;
                }
                if (msg?.type === 'ContainerRemove' && msg?.data) {
                    const id = (msg.data as any)?.containerId ?? (msg.data as any)?.ContainerId;
                    if (id) {
                        setContainers((prev) => prev.filter((x: any) => (x?.containerId ?? x?.ContainerId) !== id));
                    }
                    return;
                }

                if (msg?.type === 'AgentRuntimeUpdate' && Array.isArray(msg?.data)) {
                    setAgentRuns(msg.data);
                    return;
                }

                if (msg?.type === 'DigitalHumanRosterUpdate' && msg?.data && typeof msg.data === 'object') {
                    setDigitalHumanRoster(msg.data as any);
                    return;
                }

                if (msg?.type === 'BrainFinished' && msg?.data) {
                    const finished = Boolean((msg.data as any)?.brain_finished);
                    setBrainFinished(finished);
                    if (finished) {
                        setProjectStatus('决策结束');
                    } else {
                        setProjectStatus('正在运行');
                    }
                    return;
                }

                if (msg?.type === 'UserMessage' && msg?.data) {
                    const d = msg.data as any;
                    const personaName = String(d?.persona_name ?? '');
                    const avatarFile = String(d?.avatar_file ?? '');
                    const umsg = String(d?.message ?? '');
                    if (umsg) {
                        setChatMessages(prev => [...prev, {
                            role: 'system' as const,
                            text: umsg,
                            ts: new Date().toLocaleTimeString(),
                            persona_name: personaName,
                            avatar_file: avatarFile,
                        }]);
                    }
                    return;
                }

                if (msg?.type === 'TeamMessage' && msg?.data) {
                    const d = msg.data as any;
                    const personaName = String(d?.persona_name ?? '');
                    const avatarFile = String(d?.avatar_file ?? '');
                    const tmsg = String(d?.message ?? '');
                    if (tmsg) {
                        setChatMessages(prev => [...prev, {
                            role: 'system' as const,
                            text: `@all ${tmsg}`,
                            ts: new Date().toLocaleTimeString(),
                            persona_name: personaName,
                            avatar_file: avatarFile,
                        }]);
                    }
                    return;
                }

                if (msg?.type === 'TokenUsage' && msg?.data) {
                    const d = msg.data as any;
                    setTokenUsage({
                        prompt_tokens: Number(d?.prompt_tokens ?? 0),
                        completion_tokens: Number(d?.completion_tokens ?? 0),
                        total_tokens: Number(d?.total_tokens ?? 0),
                        agents: Array.isArray(d?.agents) ? d.agents : [],
                    });
                    return;
                }

                if (msg?.type === 'BrainContextBreakdown' && msg?.data) {
                    const d = msg.data as any;
                    setContextBreakdown({
                        sections: Array.isArray(d?.sections) ? d.sections : [],
                        total: Number(d?.total ?? 0),
                        max_context: Number(d?.max_context ?? 0),
                    });
                    return;
                }

                if (msg?.type === 'ExploitIdeaAdd' && msg?.data) {
                    const e = msg.data as any;
                    const id = e?.exploitIdeaId ?? e?.ExploitIdeaId;
                    if (id) {
                        setExploitIdeas((prev) => {
                            const exists = prev.some((x: any) => (x?.exploitIdeaId ?? x?.ExploitIdeaId) === id);
                            if (exists) return prev;
                            return [e, ...prev];
                        });
                    }
                    return;
                }
                if (msg?.type === 'ExploitIdeaUpdate' && msg?.data) {
                    const e = msg.data as any;
                    const id = e?.exploitIdeaId ?? e?.ExploitIdeaId;
                    if (id) {
                        setExploitIdeas((prev) => {
                            const out = prev.map((x: any) => ((x?.exploitIdeaId ?? x?.ExploitIdeaId) === id ? e : x));
                            const exists = out.some((x: any) => (x?.exploitIdeaId ?? x?.ExploitIdeaId) === id);
                            return exists ? out : [e, ...prev];
                        });
                    }
                    return;
                }

                if (msg?.type === 'ExploitChainAdd' && msg?.data) {
                    const e = msg.data as any;
                    const id = e?.exploit_chain_id ?? e?.ExploitChainId;
                    if (id) {
                        setExploitChains((prev) => {
                            const exists = prev.some((x: any) => (x?.exploit_chain_id ?? x?.ExploitChainId) === id);
                            if (exists) return prev;
                            return [e, ...prev];
                        });
                    }
                    return;
                }
                if (msg?.type === 'ExploitChainUpdate' && msg?.data) {
                    const e = msg.data as any;
                    const id = e?.exploit_chain_id ?? e?.ExploitChainId;
                    if (id) {
                        setExploitChains((prev) => {
                            const out = prev.map((x: any) => ((x?.exploit_chain_id ?? x?.ExploitChainId) === id ? e : x));
                            const exists = out.some((x: any) => (x?.exploit_chain_id ?? x?.ExploitChainId) === id);
                            return exists ? out : [e, ...prev];
                        });
                    }
                    return;
                }

                // Unknown messages: keep for debugging
                setEvents((prev) => [msg, ...prev].slice(0, 200));
            } catch {
                setEvents((prev) => [{ raw: ev.data }, ...prev].slice(0, 200));
            }
        };
        ws.onerror = () => {
            // ignore
        };
        wsRef.current = ws;
    }

    useEffect(() => {
        if (view !== 'detail' || !detailProject) return;
        connectWS(detailProject);
        (async () => {
            try {
                await refreshProjectDetail();
            } catch (e: any) {
                openDetail('json', 'Load Project Error', {
                    project: detailProject,
                    error: String(e?.message ?? e ?? 'error'),
                });
            }
        })();
        return () => {
            wsRef.current?.close();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [view, detailProject, baseURL]);

    // Wait for startup checks (init status + token validation) before rendering anything.
    if (!initChecked) {
        return (
            <div className="aix-shell cyber-bg" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
                <div style={{ textAlign: 'center', opacity: 0.5 }}>
                    <div style={{ fontSize: 18, fontWeight: 600 }}>AIxVuln</div>
                    <div style={{ fontSize: 13, marginTop: 8 }}>正在初始化…</div>
                </div>
            </div>
        );
    }

    // Init wizard: first-run setup (both web mode and Wails mode with empty credentials)
    if (needsInit && initChecked && (initStep === 'docker' || !authHeader)) {
        const handleInitSubmit = async () => {
            if (initStep === 'user') {
                if (!initUsername.trim()) { setSetupError('用户名不能为空'); return; }
                if (initPassword.length < 6) { setSetupError('密码至少6位'); return; }
                if (initPassword !== initPassword2) { setSetupError('两次密码不一致'); return; }
                setInitLoading(true);
                setSetupError('');
                try {
                    const initBase = baseURL || '';
                    const resp = await fetch(`${initBase}/init`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ username: initUsername.trim(), password: initPassword }),
                    });
                    const js = await resp.json();
                    if (!resp.ok || !js?.success) { setSetupError(js?.error || '初始化失败'); return; }
                    setToken(js.token);
                    localStorage.setItem('aix_token', js.token);
                    // Check if docker images need building.
                    const needsDocker = !dockerImages['aisandbox'] || !dockerImages['java_env'];
                    if (needsDocker) {
                        setInitStep('docker');
                    } else {
                        setNeedsInit(false);
                    }
                } catch (e: any) {
                    setSetupError(e?.message || '网络错误');
                } finally {
                    setInitLoading(false);
                }
            }
        };

        const streamDockerCmd = async (imageName: string, endpoint: string, label: string) => {
            setDockerBuilding(prev => ({ ...prev, [imageName]: true }));
            setDockerBuildOutput(prev => ({ ...prev, [imageName]: `${label}中，请稍候…` }));
            try {
                const initBase = baseURL || '';
                const headers: Record<string, string> = {};
                if (authHeader) headers['Authorization'] = authHeader;
                else if (token) headers['Authorization'] = 'Bearer ' + token;
                let pullUrl = `${initBase}/${endpoint}/${imageName}`;
                if (endpoint === 'docker_pull' && customRegistry.trim()) {
                    pullUrl += `?registry=${encodeURIComponent(customRegistry.trim())}`;
                }
                const resp = await fetch(pullUrl, { method: 'POST', headers });
                if (!resp.ok) {
                    let errMsg = `${label}失败 (HTTP ${resp.status})`;
                    try { const js = await resp.json(); errMsg = js?.error || errMsg; } catch {}
                    setDockerBuildOutput(prev => ({ ...prev, [imageName]: errMsg + (endpoint.includes('pull') ? '\n建议尝试「本地构建」方式。' : '') }));
                    return;
                }
                const reader = resp.body?.getReader();
                if (!reader) { setDockerBuildOutput(prev => ({ ...prev, [imageName]: `${label}失败: 无法读取响应流` })); return; }
                const decoder = new TextDecoder();
                let buf = '';
                let lines: string[] = [];
                let hasError = false;
                const MAX_LINES = 50;
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    buf += decoder.decode(value, { stream: true });
                    const parts = buf.split('\n');
                    buf = parts.pop() || '';
                    for (const part of parts) {
                        const trimmed = part.trim();
                        if (!trimmed || !trimmed.startsWith('data: ')) continue;
                        try {
                            const evt = JSON.parse(trimmed.slice(6));
                            if (evt.line) {
                                lines.push(evt.line);
                                if (lines.length > MAX_LINES) lines = lines.slice(-MAX_LINES);
                                setDockerBuildOutput(prev => ({ ...prev, [imageName]: lines.join('\n') }));
                            }
                            if (evt.done) {
                                if (evt.error) {
                                    hasError = true;
                                    lines.push(`\n❌ ${label}失败: ${evt.error}`);
                                    if (endpoint.includes('pull')) lines.push('建议尝试「本地构建」方式。');
                                } else {
                                    lines.push(`\n✅ ${label}成功！`);
                                    setDockerImages(prev => ({ ...prev, [imageName]: true }));
                                }
                                setDockerBuildOutput(prev => ({ ...prev, [imageName]: lines.join('\n') }));
                            }
                        } catch {}
                    }
                }
                if (!hasError && !lines.some(l => l.includes('成功'))) {
                    lines.push(`\n✅ ${label}成功！`);
                    setDockerImages(prev => ({ ...prev, [imageName]: true }));
                    setDockerBuildOutput(prev => ({ ...prev, [imageName]: lines.join('\n') }));
                }
            } catch (e: any) {
                setDockerBuildOutput(prev => ({ ...prev, [imageName]: `${label}失败: ${e?.message || ''}` + (endpoint.includes('pull') ? '\n建议尝试「本地构建」方式。' : '') }));
            } finally {
                setDockerBuilding(prev => ({ ...prev, [imageName]: false }));
            }
        };

        const handleDockerPull = (imageName: string) => streamDockerCmd(imageName, 'docker_pull', '拉取镜像');
        const handleDockerBuild = (imageName: string) => streamDockerCmd(imageName, 'docker_build', '本地构建');

        const registryPrefix = customRegistry.trim() ? customRegistry.trim().replace(/\/+$/, '') + '/' : 'aixvuln/';
        const remoteImageNames: Record<string, string> = { aisandbox: `${registryPrefix}aisandbox`, java_env: `${registryPrefix}java_env` };

        return (
            <div className="aix-shell cyber-bg" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
                <Card className="w-full max-w-[560px] mx-4">
                    <CardHeader className="text-center">
                        <CardTitle className="text-2xl">AIxVuln 初始化设置</CardTitle>
                        <div className="text-sm text-muted-foreground mt-1">
                            {initStep === 'user' ? '首次启动，请设置管理员账户' : 'Docker 镜像引导'}
                        </div>
                    </CardHeader>
                    <CardContent>
                        {initStep === 'user' ? (
                            <div className="space-y-4">
                                <div className="space-y-2">
                                    <label className="text-sm font-medium">用户名</label>
                                    <Input placeholder="设置管理员用户名" value={initUsername} onChange={e => setInitUsername(e.target.value)} autoFocus />
                                </div>
                                <div className="space-y-2">
                                    <label className="text-sm font-medium">密码</label>
                                    <Input type="password" placeholder="设置密码（至少6位）" value={initPassword} onChange={e => setInitPassword(e.target.value)} />
                                </div>
                                <div className="space-y-2">
                                    <label className="text-sm font-medium">确认密码</label>
                                    <Input type="password" placeholder="再次输入密码" value={initPassword2} onChange={e => setInitPassword2(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') handleInitSubmit(); }} />
                                </div>
                                {setupError && <div className="text-sm text-destructive text-center">{setupError}</div>}
                                <Button className="w-full" disabled={initLoading} onClick={handleInitSubmit}>
                                    {initLoading ? '创建中…' : '创建管理员账户'}
                                </Button>
                            </div>
                        ) : (
                            <div className="space-y-4">
                                <div className="text-xs text-muted-foreground mb-2">
                                    系统需要以下 Docker 镜像才能正常运行。推荐使用<strong>「拉取镜像」</strong>方式（更快），拉取失败时再使用「本地构建」。
                                </div>
                                <div className="rounded-lg border border-border p-3 space-y-1.5 mb-1">
                                    <label className="text-sm font-medium">自定义镜像仓库（可选）</label>
                                    <Input
                                        placeholder="例如：registry.cn-hangzhou.aliyuncs.com/yourns"
                                        value={customRegistry}
                                        onChange={e => setCustomRegistry(e.target.value)}
                                    />
                                    <div className="text-[11px] text-muted-foreground">
                                        留空则从 Docker Hub（aixvuln/）拉取。填写后将从指定仓库拉取，例如填写 <code>registry.cn-hangzhou.aliyuncs.com/myns</code> 则拉取 <code>registry.cn-hangzhou.aliyuncs.com/myns/aisandbox</code>
                                    </div>
                                </div>
                                {(['aisandbox', 'java_env'] as const).map(img => (
                                    <div key={img} className="rounded-lg border border-border p-4 space-y-2">
                                        <div className="flex items-center justify-between">
                                            <div>
                                                <span className="font-semibold text-sm">{img}</span>
                                                <span className="ml-2 text-xs text-muted-foreground">
                                                    {img === 'aisandbox' ? '攻击/验证沙箱' : 'Java 多版本运行环境'}
                                                </span>
                                            </div>
                                            {dockerImages[img] && <Badge variant="success">已就绪</Badge>}
                                        </div>
                                        {!dockerImages[img] && (
                                            <div className="space-y-1.5">
                                                <div className="text-[11px] text-muted-foreground">
                                                    远程镜像：<code className="text-xs">{remoteImageNames[img]}</code>
                                                </div>
                                                <div className="flex gap-2">
                                                    <Button size="sm" disabled={!!dockerBuilding[img]} onClick={() => handleDockerPull(img)}>
                                                        {dockerBuilding[img] ? '处理中…' : '拉取镜像（推荐）'}
                                                    </Button>
                                                    <Button size="sm" variant="outline" disabled={!!dockerBuilding[img]} onClick={() => handleDockerBuild(img)}>
                                                        {dockerBuilding[img] ? '处理中…' : '本地构建'}
                                                    </Button>
                                                </div>
                                            </div>
                                        )}
                                        {dockerBuildOutput[img] && (
                                            <pre className="text-xs bg-background/50 rounded p-2 max-h-[120px] overflow-auto whitespace-pre-wrap break-words">{dockerBuildOutput[img]}</pre>
                                        )}
                                    </div>
                                ))}
                                <Button className="w-full" onClick={() => setNeedsInit(false)}>
                                    {dockerImages['aisandbox'] && dockerImages['java_env'] ? '完成设置，进入系统' : '跳过，稍后构建'}
                                </Button>
                            </div>
                        )}
                    </CardContent>
                </Card>
            </div>
        );
    }

    // Show login page if no valid auth (web mode or Wails mode with empty credentials)
    if (!authHeader) {
        return (
            <div className="aix-shell cyber-bg" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
                <Card className="w-full max-w-[380px] mx-4">
                    <CardHeader className="text-center">
                        <CardTitle className="text-2xl">AIxVuln</CardTitle>
                        <div className="text-sm text-muted-foreground mt-1">登录以继续</div>
                    </CardHeader>
                    <CardContent>
                        <div className="space-y-4">
                            <div className="space-y-2">
                                <label className="text-sm font-medium">用户名</label>
                                <Input
                                    placeholder="请输入用户名"
                                    value={loginUser}
                                    onChange={e => setLoginUser(e.target.value)}
                                    onKeyDown={e => { if (e.key === 'Enter') handleLogin(); }}
                                    autoFocus
                                />
                            </div>
                            <div className="space-y-2">
                                <label className="text-sm font-medium">密码</label>
                                <Input
                                    type="password"
                                    placeholder="请输入密码"
                                    value={loginPass}
                                    onChange={e => setLoginPass(e.target.value)}
                                    onKeyDown={e => { if (e.key === 'Enter') handleLogin(); }}
                                />
                            </div>
                            {loginError && (
                                <div className="text-sm text-destructive text-center">{loginError}</div>
                            )}
                            <Button className="w-full" disabled={loginLoading} onClick={handleLogin}>
                                {loginLoading ? '登录中…' : '登录'}
                            </Button>
                        </div>
                    </CardContent>
                </Card>
            </div>
        );
    }

    return (<>
        <div className="aix-shell cyber-bg">
            <Dialog open={detailOpen} onOpenChange={(v) => (v ? null : closeDetail())}>
                <DialogContent className="max-h-[85vh] overflow-auto">
                    <DialogHeader>
                        <DialogTitle>{detailTitle || '详情'}</DialogTitle>
                        <button className="aix-btn" onClick={closeDetail}>关闭</button>
                    </DialogHeader>
                    <div style={{ height: 12 }} />
                    {renderDetailContent()}
                </DialogContent>
            </Dialog>

            {toast && (
                <div
                    style={{
                        position: 'fixed', top: 24, left: '50%', transform: 'translateX(-50%)',
                        zIndex: 9999, padding: '10px 24px', borderRadius: 8,
                        fontSize: 14, fontWeight: 500, pointerEvents: 'none',
                        color: '#fff',
                        background: toast.type === 'ok' ? 'rgba(34,197,94,0.9)' : 'rgba(239,68,68,0.9)',
                        boxShadow: '0 4px 16px rgba(0,0,0,0.25)',
                        animation: 'fadeIn 0.15s ease-out',
                    }}
                >
                    {toast.msg}
                </div>
            )}

            <div className="aix-header">
                <div>
                    <div className="aix-title">AIxVuln AI驱动全流程漏洞挖掘</div>
                </div>
                <div className="aix-actions">
                    {view === 'detail' ? (
                        <>
                            {tokenUsage.total_tokens > 0 && (
                                <button
                                    className="aix-nav-pill"
                                    title={`Prompt: ${formatTokens(tokenUsage.prompt_tokens)} | Completion: ${formatTokens(tokenUsage.completion_tokens)} | Total: ${formatTokens(tokenUsage.total_tokens)}`}
                                    style={{ fontSize: '0.82rem', fontFamily: 'monospace', whiteSpace: 'nowrap' }}
                                    onClick={() => setShowTokenPanel(v => !v)}
                                >
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{width:14,height:14}}><path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>
                                    Tokens: {formatTokens(tokenUsage.total_tokens)}
                                </button>
                            )}
                            <button className="aix-nav-pill aix-nav-pill--accent" onClick={() => {
                                connectWS(detailProject);
                                refreshProjectDetail().catch(() => null);
                            }}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21.5 2v6h-6"/><path d="M21.34 15.57a10 10 0 1 1-.57-8.38"/></svg>
                                刷新详情
                            </button>
                            <button className="aix-nav-pill" onClick={leaveProject}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m15 18-6-6 6-6"/></svg>
                                返回列表
                            </button>
                        </>
                    ) : view === 'settings' || view === 'digital_humans' || view === 'report_templates' ? (
                        <>
                            <button className="aix-nav-pill" onClick={() => { window.location.hash = '#/'; }}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m15 18-6-6 6-6"/></svg>
                                返回列表
                            </button>
                        </>
                    ) : (
                        <>
                            <button className="aix-nav-pill aix-nav-pill--accent" onClick={refreshProjects}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21.5 2v6h-6"/><path d="M21.34 15.57a10 10 0 1 1-.57-8.38"/></svg>
                                刷新
                            </button>
                            <div className="aix-nav-divider" />
                            <button className="aix-nav-pill" onClick={() => { window.location.hash = '#/digital_humans'; }}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
                                数字人
                            </button>
                            <button className="aix-nav-pill" onClick={() => { window.location.hash = '#/report_templates'; }}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
                                报告模板
                            </button>
                            <button className="aix-nav-pill" onClick={() => { window.location.hash = '#/settings'; }}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
                                设置
                            </button>
                        </>
                    )}
                    {token && (
                        <>
                            <div className="aix-nav-divider" />
                            <button className="aix-nav-pill aix-nav-pill--muted" onClick={handleLogout}>
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
                                退出
                            </button>
                        </>
                    )}
                </div>
            </div>

            {showTokenPanel && tokenUsage.total_tokens > 0 && (() => {
                const agents = tokenUsage.agents ?? [];
                const maxTotal = Math.max(...agents.map(a => a.total_tokens), 1);
                const barColors = ['#6366f1','#22d3ee','#f59e0b','#10b981','#f43f5e','#8b5cf6','#ec4899','#14b8a6','#ef4444','#3b82f6'];
                return (
                    <div style={{ position:'fixed', inset:0, zIndex:9999, display:'flex', alignItems:'center', justifyContent:'center', background:'rgba(0,0,0,0.55)', backdropFilter:'blur(4px)' }} onClick={() => setShowTokenPanel(false)}>
                        <div style={{ background:'#1a1a2e', borderRadius:16, padding:'28px 32px', minWidth:520, maxWidth:'90vw', maxHeight:'85vh', overflow:'auto', boxShadow:'0 8px 32px rgba(0,0,0,0.5)', border:'1px solid rgba(255,255,255,0.08)' }} onClick={e => e.stopPropagation()}>
                            <div style={{ display:'flex', justifyContent:'space-between', alignItems:'center', marginBottom:20 }}>
                                <h2 style={{ margin:0, fontSize:'1.15rem', fontWeight:600, color:'#e2e8f0' }}>Token 用量统计</h2>
                                <button onClick={() => setShowTokenPanel(false)} style={{ background:'none', border:'none', color:'#94a3b8', cursor:'pointer', fontSize:'1.2rem', padding:'4px 8px' }}>&times;</button>
                            </div>
                            <div style={{ display:'grid', gridTemplateColumns:'repeat(3,1fr)', gap:12, marginBottom:24 }}>
                                <div style={{ background:'rgba(99,102,241,0.12)', borderRadius:10, padding:'14px 16px', textAlign:'center' }}>
                                    <div style={{ fontSize:'0.75rem', color:'#94a3b8', marginBottom:4 }}>Prompt</div>
                                    <div style={{ fontSize:'1.2rem', fontWeight:700, color:'#818cf8', fontFamily:'monospace' }}>{formatTokens(tokenUsage.prompt_tokens)}</div>
                                </div>
                                <div style={{ background:'rgba(34,211,238,0.12)', borderRadius:10, padding:'14px 16px', textAlign:'center' }}>
                                    <div style={{ fontSize:'0.75rem', color:'#94a3b8', marginBottom:4 }}>Completion</div>
                                    <div style={{ fontSize:'1.2rem', fontWeight:700, color:'#22d3ee', fontFamily:'monospace' }}>{formatTokens(tokenUsage.completion_tokens)}</div>
                                </div>
                                <div style={{ background:'rgba(255,255,255,0.06)', borderRadius:10, padding:'14px 16px', textAlign:'center' }}>
                                    <div style={{ fontSize:'0.75rem', color:'#94a3b8', marginBottom:4 }}>Total</div>
                                    <div style={{ fontSize:'1.2rem', fontWeight:700, color:'#e2e8f0', fontFamily:'monospace' }}>{formatTokens(tokenUsage.total_tokens)}</div>
                                </div>
                            </div>
                            {agents.length > 0 && (
                                <>
                                    <div style={{ fontSize:'0.85rem', color:'#94a3b8', marginBottom:12, fontWeight:500 }}>各数字人 / 决策大脑用量</div>
                                    <div style={{ display:'flex', flexDirection:'column', gap:10 }}>
                                        {agents.map((a, i) => {
                                            const pct = Math.max((a.total_tokens / maxTotal) * 100, 2);
                                            const promptPct = a.total_tokens > 0 ? (a.prompt_tokens / a.total_tokens) * 100 : 50;
                                            const color = barColors[i % barColors.length];
                                            return (
                                                <div key={a.label} style={{ display:'flex', alignItems:'center', gap:12 }}>
                                                    <div style={{ width:80, fontSize:'0.8rem', color:'#cbd5e1', textAlign:'right', flexShrink:0, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }} title={a.label}>{a.label}</div>
                                                    <div style={{ flex:1, position:'relative', height:22, background:'rgba(255,255,255,0.04)', borderRadius:6, overflow:'hidden' }}>
                                                        <div style={{ position:'absolute', left:0, top:0, height:'100%', width:`${pct}%`, borderRadius:6, display:'flex', overflow:'hidden' }}>
                                                            <div style={{ width:`${promptPct}%`, height:'100%', background:color, opacity:0.9 }} />
                                                            <div style={{ flex:1, height:'100%', background:color, opacity:0.5 }} />
                                                        </div>
                                                    </div>
                                                    <div style={{ width:80, fontSize:'0.75rem', color:'#94a3b8', fontFamily:'monospace', textAlign:'right', flexShrink:0 }}>{formatTokens(a.total_tokens)}</div>
                                                </div>
                                            );
                                        })}
                                    </div>
                                    <div style={{ display:'flex', gap:16, marginTop:14, fontSize:'0.72rem', color:'#64748b', justifyContent:'flex-end' }}>
                                        <span style={{ display:'flex', alignItems:'center', gap:4 }}><span style={{ display:'inline-block', width:10, height:10, borderRadius:2, background:'#6366f1', opacity:0.9 }} /> Prompt</span>
                                        <span style={{ display:'flex', alignItems:'center', gap:4 }}><span style={{ display:'inline-block', width:10, height:10, borderRadius:2, background:'#6366f1', opacity:0.5 }} /> Completion</span>
                                    </div>
                                </>
                            )}
                        </div>
                    </div>
                );
            })()}

            <div className={view === 'detail' ? 'aix-grid aix-grid--detail' : 'aix-grid'}>
                {view === 'settings' ? (() => {
                    const sectionDescriptions: Record<string, string> = {
                        misc: '全局通用配置（消息长度限制、重试次数、数据目录等）',
                        decision: '决策大脑专用配置（可覆盖 main_setting 中的 LLM 和并发配置）',
                        main_setting: '全局默认 LLM 配置（各 Agent section 未配置时的 fallback）',
                        ops: '运维 Agent 专用配置（可覆盖 main_setting）',
                        analyze: '代码审计 Agent 专用配置（可覆盖 main_setting）',
                        verifier: '漏洞验证 Agent 专用配置（可覆盖 main_setting）',
                        report: '报告编写 Agent 专用配置（可覆盖 main_setting）',
                        overview: '项目概览 Agent 专用配置（可覆盖 main_setting）',
                    };
                    const keyDescriptions: Record<string, string> = {
                        MaxContext: '最大Context大小（KB），超过将触发压缩机制',
                        MessageMaximum: '单条消息最大长度（字符），超过将截断',
                        MaxTryCount: 'API请求错误的最大重试次数',
                        DATA_DIR: '数据存储目录',
                        MaxRequest: '单个API-KEY的最大并发请求数',
                        FeiShuAPI: '飞书机器人 Webhook API 地址',
                        USER_AGENT: 'HTTP请求的 User-Agent 头',
                        STREAM: '是否启用流式响应（true/false）',
                        API_MODE: 'API模式：chat = Chat Completions API, responses = Responses API',
                        BASE_URL: 'LLM API 基础地址',
                        OPENAI_API_KEY: 'API密钥（多个用 |-| 分隔）',
                        MODEL: '模型名称',
                    };
                    const sectionKeys: Record<string, string[]> = {
                        misc: ['MessageMaximum', 'MaxTryCount', 'DATA_DIR', 'FeiShuAPI'],
                        main_setting: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                        decision: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                        ops: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                        analyze: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                        verifier: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                        report: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                        overview: ['BASE_URL', 'OPENAI_API_KEY', 'MODEL', 'MaxContext', 'MaxRequest', 'USER_AGENT', 'STREAM', 'API_MODE'],
                    };
                    const requiredKeys = new Set(['main_setting:BASE_URL', 'main_setting:OPENAI_API_KEY', 'main_setting:MODEL']);
                    const isRequired = (section: string, key: string) => requiredKeys.has(`${section}:${key}`);
                    const predefinedSections = Object.keys(sectionKeys);
                    const availableSectionsToAdd = predefinedSections.filter(s => !configDraft[s]);
                    return (
                <Card className="col-span-full">
                    <CardHeader>
                        <div className="flex flex-col sm:flex-row sm:items-center justify-between w-full gap-3">
                            <div>
                                <CardTitle>系统设置</CardTitle>
                                <div className="mt-1 text-xs text-muted-foreground">配置存储于 SQLite · 首次启动自动生成默认值 · 保存后部分配置需要重启生效</div>
                            </div>
                            <div className="flex items-center gap-2 shrink-0">
                                <Button size="sm" variant="outline" onClick={fetchConfig} disabled={configLoading}>{configLoading ? '加载中…' : '重新加载'}</Button>
                                <Button size="sm" onClick={saveConfig} disabled={configSaving}>{configSaving ? '保存中…' : '保存配置'}</Button>
                            </div>
                        </div>
                    </CardHeader>
                    <CardContent>
                        <div className="space-y-6">
                            {Object.keys(configDraft).length === 0 && !configLoading ? (
                                <div className="text-sm text-muted-foreground p-4 text-center">暂无配置数据，请点击"重新加载"</div>
                            ) : null}
                            {Object.entries(configDraft).map(([section, kv]) => {
                                const allKeys = sectionKeys[section] || [];
                                const existingKeys = Object.keys(kv);
                                const availableKeys = allKeys.filter(k => !existingKeys.includes(k));
                                return (
                                    <div key={section} className="rounded-lg border border-border bg-background/30 overflow-hidden">
                                        <div className="flex items-center justify-between px-4 py-3 bg-muted/30 border-b border-border">
                                            <div>
                                                <span className="font-semibold text-sm">[{section}]</span>
                                                {sectionDescriptions[section] && (
                                                    <span className="ml-2 text-xs text-muted-foreground">{sectionDescriptions[section]}</span>
                                                )}
                                            </div>
                                            <Button size="sm" variant="destructive" onClick={() => { if (confirm(`确定删除整个 [${section}] 配置段？`)) deleteConfigSection(section); }}>删除段</Button>
                                        </div>
                                        <div className="p-4 space-y-3">
                                            {Object.entries(kv).map(([key, value]) => {
                                                const models = modelOptions[section] || [];
                                                const isModelKey = key === 'MODEL';
                                                const hasModels = isModelKey && models.length > 0;
                                                const isCustomModel = isModelKey && value.trim() !== '' && !models.includes(value);
                                                return (
                                                <div key={key} className="aix-config-row flex items-start gap-2">
                                                    <div className="aix-config-label w-[200px] shrink-0">
                                                        <div className="text-sm font-medium">{key}{isRequired(section, key) && <span className="text-destructive ml-1">*</span>}</div>
                                                        {keyDescriptions[key] && (
                                                            <div className="text-[11px] text-muted-foreground leading-tight mt-0.5">{keyDescriptions[key]}</div>
                                                        )}
                                                    </div>
                                                    {isModelKey ? (
                                                        <div className="flex-1 flex items-center gap-2 min-w-0">
                                                            {hasModels && (
                                                                <select
                                                                    className="aix-select flex-1 font-mono"
                                                                    value={isCustomModel ? '__custom__' : value}
                                                                    onChange={e => {
                                                                        if (e.target.value === '__custom__') return;
                                                                        updateConfigDraft(section, key, e.target.value);
                                                                    }}
                                                                >
                                                                    <option value="">选择模型…</option>
                                                                    {models.map(m => (
                                                                        <option key={m} value={m}>{m}</option>
                                                                    ))}
                                                                    <option value="__custom__">✏ 自定义输入</option>
                                                                </select>
                                                            )}
                                                            {(!hasModels || isCustomModel) && (
                                                                <Input
                                                                    className={`flex-1 font-mono text-sm ${isRequired(section, key) && !value.trim() ? 'border-destructive' : ''}`}
                                                                    value={value}
                                                                    placeholder={isRequired(section, key) ? '必填 — 输入模型名称' : '输入模型名称'}
                                                                    onChange={e => updateConfigDraft(section, key, e.target.value)}
                                                                />
                                                            )}
                                                            <Button
                                                                size="sm"
                                                                variant="outline"
                                                                className="shrink-0 text-xs"
                                                                disabled={modelLoading[section] || !kv['BASE_URL']?.trim() || !kv['OPENAI_API_KEY']?.trim()}
                                                                onClick={() => fetchModels(section)}
                                                            >
                                                                {modelLoading[section] ? '获取中…' : '获取模型'}
                                                            </Button>
                                                        </div>
                                                    ) : (
                                                        <Input
                                                            className={`flex-1 font-mono text-sm ${isRequired(section, key) && !value.trim() ? 'border-destructive' : ''}`}
                                                            value={value}
                                                            type={key === 'OPENAI_API_KEY' ? 'password' : 'text'}
                                                            placeholder={isRequired(section, key) ? '必填' : ''}
                                                            onChange={e => updateConfigDraft(section, key, e.target.value)}
                                                            onBlur={() => {
                                                                if ((key === 'BASE_URL' || key === 'OPENAI_API_KEY') && kv['BASE_URL']?.trim() && kv['OPENAI_API_KEY']?.trim()) {
                                                                    fetchModels(section);
                                                                }
                                                            }}
                                                        />
                                                    )}
                                                    <Button size="sm" variant="ghost" className="text-destructive shrink-0" onClick={() => deleteConfigKey(section, key)}>✕</Button>
                                                </div>
                                                );
                                            })}
                                            {availableKeys.length > 0 && (
                                            <div className="aix-config-add-row flex items-center gap-2 pt-2 border-t border-border/50">
                                                <select
                                                    className="aix-select w-[220px]"
                                                    value={newKeyInputs[section]?.key || ''}
                                                    onChange={e => setNewKeyInputs(prev => ({ ...prev, [section]: { key: e.target.value, value: prev[section]?.value || '' } }))}
                                                >
                                                    <option value="">选择配置项…</option>
                                                    {availableKeys.map(k => (
                                                        <option key={k} value={k}>{k}{keyDescriptions[k] ? ` — ${keyDescriptions[k]}` : ''}</option>
                                                    ))}
                                                </select>
                                                <Input
                                                    className="flex-1 text-sm"
                                                    placeholder="值"
                                                    value={newKeyInputs[section]?.value || ''}
                                                    onChange={e => setNewKeyInputs(prev => ({ ...prev, [section]: { key: prev[section]?.key || '', value: e.target.value } }))}
                                                    onKeyDown={e => { if (e.key === 'Enter') addConfigKey(section); }}
                                                />
                                                <Button size="sm" variant="outline" onClick={() => addConfigKey(section)}>添加</Button>
                                            </div>
                                            )}
                                        </div>
                                    </div>
                                );
                            })}
                            {availableSectionsToAdd.length > 0 && (
                            <div className="flex items-center gap-2 pt-2 border-t border-border">
                                <select
                                    className="aix-select w-[280px]"
                                    value={newSectionName}
                                    onChange={e => setNewSectionName(e.target.value)}
                                >
                                    <option value="">选择配置段…</option>
                                    {availableSectionsToAdd.map(s => (
                                        <option key={s} value={s}>[{s}]{sectionDescriptions[s] ? ` — ${sectionDescriptions[s]}` : ''}</option>
                                    ))}
                                </select>
                                <Button size="sm" variant="outline" disabled={!newSectionName} onClick={() => addConfigSection(newSectionName)}>添加配置段</Button>
                            </div>
                            )}
                        </div>
                    </CardContent>
                </Card>
                    );
                })()
                : view === 'digital_humans' ? (() => {
                    const agentTypeLabels: Record<string, string> = {
                        'Agent-Ops-OpsCommonAgent': '环境运维 (Ops)',
                        'Agent-Ops-OpsEnvScoutAgent': '环境侦察 (EnvScout)',
                        'Agent-Analyze-AnalyzeCommonAgent': '代码审计 (Analyze)',
                        'Agent-Verifier-VerifierCommonAgent': '漏洞验证 (Verifier)',
                        'Agent-Report-ReportCommonAgent': '报告编写 (Report)',
                    };
                    const agentTypes = Object.keys(agentTypeLabels);
                    const emptyDh = () => ({
                        id: crypto.randomUUID(),
                        agent_type: agentTypes[0],
                        persona_name: '',
                        gender: '男',
                        avatar_file: '',
                        personality: '',
                        age: 25,
                        extra_sys_prompt: '',
                    });
                    return (
                <Card className="col-span-full">
                    <CardHeader>
                        <div className="flex flex-col sm:flex-row sm:items-center justify-between w-full gap-3">
                            <div>
                                <CardTitle>数字人管理</CardTitle>
                                <div className="mt-1 text-xs text-muted-foreground">管理 Agent 数字人角色 · 修改后重启项目生效</div>
                            </div>
                            <div className="flex items-center gap-2 shrink-0">
                                <Button size="sm" variant="outline" onClick={fetchDigitalHumans} disabled={dhLoading}>{dhLoading ? '加载中…' : '刷新'}</Button>
                                <Button size="sm" onClick={() => setDhEditing(emptyDh())}>新增数字人</Button>
                            </div>
                        </div>
                    </CardHeader>
                    <CardContent>
                        {dhEditing && (
                            <div className="mb-6 rounded-lg border border-primary/30 bg-background/50 p-4 space-y-3">
                                <div className="text-sm font-semibold mb-2">{digitalHumans.some((d: any) => d.id === dhEditing.id) ? '编辑数字人' : '新增数字人'}</div>
                                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                                    <div>
                                        <label className="text-xs text-muted-foreground">姓名 <span className="text-destructive">*</span></label>
                                        <Input className="mt-1" value={dhEditing.persona_name} onChange={e => setDhEditing({...dhEditing, persona_name: e.target.value})} placeholder="角色姓名" />
                                    </div>
                                    <div>
                                        <label className="text-xs text-muted-foreground">Agent 类型 <span className="text-destructive">*</span></label>
                                        <select className="aix-select w-full mt-1" value={dhEditing.agent_type} onChange={e => setDhEditing({...dhEditing, agent_type: e.target.value})}>
                                            {agentTypes.map(t => <option key={t} value={t}>{agentTypeLabels[t]}</option>)}
                                        </select>
                                    </div>
                                    <div>
                                        <label className="text-xs text-muted-foreground">性别</label>
                                        <select className="aix-select w-full mt-1" value={dhEditing.gender} onChange={e => setDhEditing({...dhEditing, gender: e.target.value})}>
                                            <option value="男">男</option>
                                            <option value="女">女</option>
                                        </select>
                                    </div>
                                    <div>
                                        <label className="text-xs text-muted-foreground">年龄</label>
                                        <Input className="mt-1" type="number" value={dhEditing.age} onChange={e => setDhEditing({...dhEditing, age: parseInt(e.target.value) || 0})} />
                                    </div>
                                    <div>
                                        <label className="text-xs text-muted-foreground">头像</label>
                                        <div className="mt-1 flex items-center gap-3">
                                            {dhEditing.avatar_file && (
                                                <img
                                                    src={(() => {
                                                        const f = dhEditing.avatar_file;
                                                        if (f.includes('-') && !f.includes('/')) {
                                                            try { return new URL(`./assets/avatar/${f}`, import.meta.url).toString(); } catch {}
                                                        }
                                                        return `${baseURL}/avatar/${f}`;
                                                    })()}
                                                    alt="avatar"
                                                    className="h-10 w-10 rounded-full border border-border object-cover shrink-0"
                                                    onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }}
                                                />
                                            )}
                                            <div className="flex-1 space-y-1">
                                                <Input
                                                    type="file"
                                                    accept=".png,.jpg,.jpeg,.gif,.webp"
                                                    onChange={async (e) => {
                                                        const file = e.target.files?.[0];
                                                        if (!file) return;
                                                        const fd = new FormData();
                                                        fd.append('file', file);
                                                        try {
                                                            const resp = await fetch(`${baseURL}/avatar/upload`, {
                                                                method: 'POST',
                                                                headers: { Authorization: authHeader },
                                                                body: fd,
                                                            });
                                                            const js = await resp.json();
                                                            const fname = getRespData<string>(js);
                                                            if (fname) {
                                                                setDhEditing((prev: any) => ({ ...prev, avatar_file: fname }));
                                                                showToast('头像上传成功');
                                                            } else {
                                                                showToast(js?.error || '上传失败', 'err');
                                                            }
                                                        } catch (err: any) {
                                                            showToast('上传失败: ' + (err?.message || ''), 'err');
                                                        }
                                                    }}
                                                />
                                                {dhEditing.avatar_file && (
                                                    <div className="text-[10px] text-muted-foreground truncate">{dhEditing.avatar_file}</div>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                    <div>
                                        <label className="text-xs text-muted-foreground">性格特征</label>
                                        <Input className="mt-1" value={dhEditing.personality} onChange={e => setDhEditing({...dhEditing, personality: e.target.value})} placeholder="例如：沉稳踏实、逻辑缜密" />
                                    </div>
                                </div>
                                <div>
                                    <label className="text-xs text-muted-foreground">额外系统提示词</label>
                                    <Textarea className="mt-1 min-h-[80px] text-sm" value={dhEditing.extra_sys_prompt} onChange={e => setDhEditing({...dhEditing, extra_sys_prompt: e.target.value})} placeholder="自定义该数字人的语气、风格等系统提示词" />
                                </div>
                                <div className="flex gap-2 pt-2">
                                    <Button size="sm" disabled={!dhEditing.persona_name.trim()} onClick={() => saveDigitalHumanAPI(dhEditing)}>保存</Button>
                                    <Button size="sm" variant="outline" onClick={() => setDhEditing(null)}>取消</Button>
                                </div>
                            </div>
                        )}
                        <div className="space-y-3">
                            {digitalHumans.length === 0 && !dhLoading ? (
                                <div className="text-sm text-muted-foreground p-4 text-center">暂无数字人数据</div>
                            ) : null}
                            {digitalHumans.map((dh: any) => {
                                const dhAvatarSrc = (() => {
                                    const f = dh.avatar_file;
                                    if (!f) return '';
                                    if (f.includes('-') && !f.includes('/')) {
                                        try { return new URL(`./assets/avatar/${f}`, import.meta.url).toString(); } catch {}
                                    }
                                    return `${baseURL}/avatar/${f}`;
                                })();
                                return (
                                <div key={dh.id} className="rounded-lg border border-border bg-background/30 p-4 flex items-start gap-4">
                                    {dhAvatarSrc && (
                                        <img src={dhAvatarSrc} alt={dh.persona_name} className="h-10 w-10 rounded-full border border-border object-cover shrink-0" onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }} />
                                    )}
                                    <div className="flex-1 min-w-0">
                                        <div className="flex items-center gap-2 mb-1">
                                            <span className="font-semibold text-sm">{dh.persona_name}</span>
                                            <Badge variant="secondary" className="text-[10px]">{agentTypeLabels[dh.agent_type] || dh.agent_type}</Badge>
                                            <span className="text-xs text-muted-foreground">{dh.gender} · {dh.age}岁</span>
                                        </div>
                                        {dh.personality && <div className="text-xs text-muted-foreground mb-1">性格：{dh.personality}</div>}
                                        {dh.extra_sys_prompt && <div className="text-xs text-muted-foreground/70 line-clamp-2">提示词：{dh.extra_sys_prompt}</div>}
                                    </div>
                                    <div className="flex gap-1 shrink-0">
                                        <Button size="sm" variant="outline" onClick={() => setDhEditing({...dh})}>编辑</Button>
                                        <Button size="sm" variant="destructive" onClick={() => { if (confirm(`确定删除数字人「${dh.persona_name}」？`)) deleteDigitalHumanAPI(dh.id); }}>删除</Button>
                                    </div>
                                </div>
                                );
                            })}
                        </div>
                    </CardContent>
                </Card>
                    );
                })()
                : view === 'report_templates' ? (() => {
                    const templateLabels: Record<string, string> = {
                        verifier: '验证型报告模板（含运行证据、PoC、HTTP包等）',
                        analyze: '分析型报告模板（代码审计、利用链分析）',
                    };
                    return (
                <Card className="col-span-full">
                    <CardHeader>
                        <div className="flex flex-col sm:flex-row sm:items-center justify-between w-full gap-3">
                            <div>
                                <CardTitle>报告模板管理</CardTitle>
                                <div className="mt-1 text-xs text-muted-foreground">自定义漏洞报告模板 · 存储于 data/.reportTemplate/ · 修改后立即生效</div>
                            </div>
                            <div className="flex items-center gap-2 shrink-0">
                                <Button size="sm" variant="outline" onClick={fetchReportTemplates} disabled={rtLoading}>{rtLoading ? '加载中…' : '重新加载'}</Button>
                                <Button size="sm" onClick={saveReportTemplates} disabled={rtSaving}>{rtSaving ? '保存中…' : '保存模板'}</Button>
                            </div>
                        </div>
                    </CardHeader>
                    <CardContent>
                        <div className="space-y-6">
                            {Object.entries(reportTemplatesDraft).map(([name, content]) => (
                                <div key={name} className="rounded-lg border border-border bg-background/30 overflow-hidden">
                                    <div className="px-4 py-3 bg-muted/30 border-b border-border">
                                        <span className="font-semibold text-sm">{name}</span>
                                        {templateLabels[name] && (
                                            <span className="ml-2 text-xs text-muted-foreground">{templateLabels[name]}</span>
                                        )}
                                    </div>
                                    <div className="p-4">
                                        <Textarea
                                            className="min-h-[300px] font-mono text-sm"
                                            value={content}
                                            onChange={e => setReportTemplatesDraft(prev => ({ ...prev, [name]: e.target.value }))}
                                        />
                                    </div>
                                </div>
                            ))}
                            {Object.keys(reportTemplatesDraft).length === 0 && !rtLoading && (
                                <div className="text-sm text-muted-foreground p-4 text-center">暂无模板数据</div>
                            )}
                        </div>
                    </CardContent>
                </Card>
                    );
                })()
                : view === 'home' ? (
                <>
                <Card>
                    <CardHeader>
                        <CardTitle>项目</CardTitle>
                        <Badge variant="primary">项目列表</Badge>
                    </CardHeader>
                    <CardContent>
                        <ScrollArea className="h-[50vh] rounded-lg border border-border bg-background/20">
                            <div className="p-2 space-y-2">
                                {projects.map((p) => (
                                    <div
                                        key={p}
                                        className="aix-project-item flex items-center justify-between gap-2 rounded-lg border border-border bg-background/10 px-3 py-2 hover:bg-muted/30"
                                    >
                                        <button className="flex-1 text-left min-w-0" onClick={() => enterProject(p)}>
                                            <div className="font-semibold truncate">{p}</div>
                                        </button>
                                        <div className="aix-project-btns flex shrink-0 flex-wrap gap-2">
                                            <Button size="sm" variant="secondary" disabled={!!btnLoading[`start_${p}`]} onClick={() => startProject(p)}>{btnLoading[`start_${p}`] ? '启动中…' : '启动'}</Button>
                                            <Button size="sm" variant="outline" disabled={!!btnLoading[`stop_${p}`]} onClick={() => stopProject(p)}>{btnLoading[`stop_${p}`] ? '停止中…' : '停止'}</Button>
                                            <Button size="sm" variant="default" onClick={() => enterProject(p)}>进入</Button>
                                            <Button size="sm" variant="destructive" disabled={!!btnLoading[`del_${p}`]} onClick={() => deleteProject(p)}>{btnLoading[`del_${p}`] ? '删除中…' : '删除'}</Button>
                                        </div>
                                    </div>
                                ))}
                                {projects.length === 0 ? <div className="text-sm text-muted-foreground p-2">暂无项目</div> : null}
                            </div>
                        </ScrollArea>

                        <div className="mt-3 text-xs text-muted-foreground">
                            说明：历史项目若任务内容为空，点击启动会提示“任务内容为空”。
                        </div>
                    </CardContent>
                </Card>

                <Card>
                    <CardHeader>
                        <CardTitle>创建项目</CardTitle>
                        <Badge>源码 + 任务内容</Badge>
                    </CardHeader>
                    <CardContent>
                        <div className="space-y-3">
                            <Input
                                placeholder="项目名称（可选，例如：demo-001）"
                                value={projectName}
                                onChange={(e) => setProjectName(e.target.value)}
                            />
                            <Textarea
                                placeholder="任务内容（会传给决策大脑，例如：挖掘一个未授权RCE漏洞）"
                                value={taskContent}
                                onChange={(e) => setTaskContent(e.target.value)}
                            />
                            <div>
                                <label className="text-xs text-muted-foreground mb-1 block">源码来源</label>
                                <div className="flex gap-1">
                                    {([['file', '压缩包文件'], ['git', 'Git 仓库'], ['url', '压缩包链接']] as const).map(([val, label]) => (
                                        <button
                                            key={val}
                                            className={`px-3 py-1.5 text-xs rounded-md border transition-colors ${sourceType === val ? 'bg-primary text-primary-foreground border-primary' : 'bg-background border-border text-muted-foreground hover:text-foreground'}`}
                                            onClick={() => setSourceType(val)}
                                        >
                                            {label}
                                        </button>
                                    ))}
                                </div>
                            </div>
                            {sourceType === 'file' && (
                                <Input ref={fileRef as any} type="file" accept=".zip,.tar.gz,.tgz" />
                            )}
                            {sourceType === 'git' && (
                                <Input
                                    placeholder="Git 仓库地址（例如：https://github.com/user/repo.git）"
                                    value={gitUrl}
                                    onChange={(e) => setGitUrl(e.target.value)}
                                />
                            )}
                            {sourceType === 'url' && (
                                <Input
                                    placeholder="压缩包下载链接（例如：https://example.com/source.zip）"
                                    value={fileUrl}
                                    onChange={(e) => setFileUrl(e.target.value)}
                                />
                            )}
                            <div className="flex gap-2">
                                <Button disabled={!!btnLoading.create} onClick={createProject}>
                                    {btnLoading.create ? '创建中…' : sourceType === 'file' ? '上传并创建' : sourceType === 'git' ? '克隆并创建' : '下载并创建'}
                                </Button>
                            </div>
                        </div>

                        <div style={{ display: 'none' }}>{initError}{baseURL}{events.length}</div>
                    </CardContent>
                </Card>
                </>
                ) : (
                <>
    <Card className="overflow-hidden min-w-0">
        <CardHeader>
            <div>
                <CardTitle>项目详情</CardTitle>
                <div className="mt-1 text-xs text-muted-foreground">实时状态面板 · 点击条目查看字段详情</div>
            </div>
            <div className="flex items-center gap-2">
                <Badge variant="primary">{detailProject || '-'}</Badge>
                {projectStatus ? (
                    <Badge variant={brainFinished ? 'default' : projectIsRunning ? 'warning' : 'secondary'}>{projectStatus}</Badge>
                ) : null}
            </div>
        </CardHeader>
        <CardContent className="overflow-hidden min-w-0">
            <div className="flex flex-wrap gap-2">
                <Button variant="secondary" disabled={!detailProject} onClick={() => detailProject && startProject(detailProject)}>启动</Button>
                <Button variant="outline" disabled={!detailProject} onClick={() => detailProject && stopProject(detailProject)}>停止</Button>
                {brainFinished && projectStatus === '决策结束' && (
                    <Button
                        variant="destructive"
                        disabled={!detailProject || !!btnLoading[`finish_${detailProject}`]}
                        onClick={async () => {
                            if (!detailProject) return;
                            setBtnLoading(prev => ({ ...prev, [`finish_${detailProject}`]: true }));
                            try {
                                await apiGet(`/projects/${encodeURIComponent(detailProject)}/cancel`);
                                showToast('项目已结束');
                            } catch (e: any) {
                                showToast(e?.message || '结束失败', 'err');
                            } finally {
                                setBtnLoading(prev => ({ ...prev, [`finish_${detailProject}`]: false }));
                            }
                        }}
                    >
                        {btnLoading[`finish_${detailProject}`] ? '结束中…' : '✅ 结束项目'}
                    </Button>
                )}
                <Button
                    variant="destructive"
                    disabled={!detailProject}
                    onClick={() => detailProject && deleteProject(detailProject).then(() => leaveProject())}
                >
                    删除
                </Button>
            </div>

            <div className="mt-4 grid gap-4 min-w-0 aix-detail-grid">
                <div className="grid gap-4 min-w-0 overflow-hidden">
                    <div className="aix-list-section">
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">数字人</div>
                            <Badge>{Object.values(digitalHumanRoster || {}).reduce((n, arr) => n + (Array.isArray(arr) ? arr.length : 0), 0)}</Badge>
                        </div>
                        <ScrollArea className="mt-2 h-[360px] rounded-lg border border-border bg-background/20 min-w-0">
                            <div className="p-2 space-y-2 min-w-0">
                                {Object.keys(digitalHumanRoster || {}).length === 0 ? (
                                    <div className="text-sm text-muted-foreground p-2">暂无数字人信息</div>
                                ) : null}

                                {Object.entries(digitalHumanRoster || {}).map(([toolName, list]) => {
                                    const roleLabel = (() => {
                                        const t = String(toolName || '');
                                        if (t.includes('Agent-Ops-OpsEnvScoutAgent')) return '远程环境运维工程师';
                                        if (t.includes('Agent-Ops-OpsCommonAgent')) return '环境运维专家';
                                        if (t.includes('Agent-Analyze-')) return '代码审计专家';
                                        if (t.includes('Agent-Verifier-')) return '漏洞验证专家';
                                        if (t.includes('Agent-Report-')) return '报告编写专员';
                                        return t;
                                    })();

                                    const arr = Array.isArray(list) ? list : [];
                                    const groupQueueLen = arr.length > 0 ? Number((arr[0] as any)?.queue_length ?? 0) : 0;
                                    return (
                                        <div key={toolName} className="space-y-2">
                                            <div className="text-xs font-semibold text-muted-foreground px-1 flex items-center gap-2">{roleLabel}{groupQueueLen > 0 ? <Badge variant="secondary" className="text-[10px] px-1.5 py-0">待处理 {groupQueueLen}</Badge> : null}</div>
                                            {arr.map((h: any, idx: number) => {
                                                const digitalHumanId = String(h?.digital_human_id ?? '');
                                                const personaName = String(h?.persona_name ?? '-');
                                                const gender = String(h?.gender ?? '-');
                                                const avatarFile = String(h?.avatar_file ?? '');
                                                const personality = String(h?.personality ?? '');
                                                const age = Number(h?.age ?? 0);
                                                const state = String(h?.state ?? '-');
                                                const queueLen = Number(h?.queue_length ?? 0);
                                                const agentId = String(h?.agent_id ?? '');
                                                const run = agentId ? agentRuns.find((x: any) => String(x?.AgentID ?? '') === agentId) : null;
                                                const runState = run?.RunState;
                                                const runTask = run?.RUNTask;
                                                const runSummary = run?.RUNSummary;

                                                const avatarSrc = (() => {
                                                    const file = avatarFile && avatarFile !== '-' ? avatarFile : 'system.png';
                                                    try {
                                                        return new URL(`./assets/avatar/${file}`, import.meta.url).toString();
                                                    } catch (e) {
                                                        return new URL(`./assets/avatar/system.png`, import.meta.url).toString();
                                                    }
                                                })();

                                                return (
                                                    <div
                                                        key={`${toolName}-${personaName}-${idx}`}
                                                        className="rounded-lg border border-border bg-background/10 px-3 py-2 hover:bg-muted/30 cursor-pointer overflow-hidden min-w-0"
                                                        onClick={() => openDetail('digitalHumanProfile', `${personaName}`, {
                                                            digital_human_id: digitalHumanId,
                                                            persona_name: personaName,
                                                            gender,
                                                            personality,
                                                            age,
                                                            role: roleLabel,
                                                            avatar_file: avatarFile,
                                                            avatar_src: avatarSrc,
                                                            state,
                                                            queue_length: queueLen,
                                                            agent_id: agentId,
                                                            RunState: runState,
                                                            RUNTask: runTask,
                                                            RUNSummary: runSummary,
                                                            last_summary: h?.last_summary ?? '',
                                                            last_task: h?.last_task ?? '',
                                                        })}
                                                    >
                                                        <div className="flex items-center justify-between gap-2 min-w-0">
                                                            <div className="flex items-center gap-2 min-w-0 overflow-hidden">
                                                                <img
                                                                    src={avatarSrc}
                                                                    alt={avatarFile || personaName}
                                                                    className="h-8 w-8 rounded-full border border-border object-cover"
                                                                />
                                                                <div className="min-w-0">
                                                                    <div className="font-semibold text-sm truncate">{personaName}-{roleLabel}</div>
                                                                </div>
                                                            </div>
                                                            <div className="flex items-center gap-2">
                                                                <Badge variant={state === 'busy' ? 'warning' : 'secondary'}>{state === 'busy' ? '忙碌' : '空闲'}</Badge>
                                                            </div>
                                                        </div>
                                                        {(() => {
                                                            const ctxSize = Number(h?.context_size ?? 0);
                                                            const maxCtx = Number(h?.max_context ?? 0);
                                                            if (maxCtx <= 0) return null;
                                                            const pct = Math.min(100, (ctxSize / maxCtx) * 100);
                                                            const barColor = pct > 85 ? '#ef4444' : pct > 60 ? '#f59e0b' : '#10b981';
                                                            return (
                                                                <div className="mt-1.5 space-y-1">
                                                                    <div className="flex items-center justify-between gap-2">
                                                                        {runState ? <Badge variant={badgeVariantForRunState(runState)}>{String(runState)}</Badge> : <span />}
                                                                        <span className="text-[10px] tabular-nums text-muted-foreground">{ctxSize.toLocaleString()} / {maxCtx.toLocaleString()} tokens</span>
                                                                    </div>
                                                                    <div style={{ height: 4, borderRadius: 2, background: 'rgba(255,255,255,0.06)', overflow: 'hidden' }}>
                                                                        <div style={{ height: '100%', borderRadius: 2, width: `${pct}%`, background: barColor, transition: 'width 0.5s ease' }} />
                                                                    </div>
                                                                </div>
                                                            );
                                                        })()}
                                                        {state === 'idle' && (h?.last_summary || h?.last_task) ? (
                                                            <div className="mt-1 text-xs text-muted-foreground line-clamp-2" style={{ overflowWrap: 'anywhere' }}>上次：{String(h?.last_summary || h?.last_task || '')}</div>
                                                        ) : null}
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    );
                                })}
                            </div>
                        </ScrollArea>
                    </div>

                    <div className="aix-list-section">
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">容器</div>
                            <Badge>{containers.length}</Badge>
                        </div>
                        <ScrollArea className="mt-2 h-[220px] rounded-lg border border-border bg-background/20 min-w-0">
                            <div className="p-2 space-y-2 min-w-0">
                                {containers.map((c, idx) => (
                                    <div
                                        key={c?.containerId ?? c?.ContainerId ?? idx}
                                        className="rounded-lg border border-border bg-background/10 px-3 py-2 hover:bg-muted/30 cursor-pointer min-w-0 overflow-hidden"
                                        onClick={() => openDetail('container', String(c?.containerId ?? c?.ContainerId ?? 'container'), c)}
                                    >
                                        <div className="font-semibold text-sm truncate">{String(c?.image ?? c?.Image ?? '-')}</div>
                                        <div className="mt-1 text-xs text-muted-foreground truncate">containerId：{String(c?.containerId ?? c?.ContainerId ?? '-')}</div>
                                        <div className="mt-1 text-xs text-muted-foreground truncate">containerIP：{String(c?.containerIP ?? c?.ContainerIP ?? '-')}</div>
                                        {(() => { const wp = c?.webPort ?? c?.WebPort; return Array.isArray(wp) && wp.length > 0 ? <div className="mt-1 text-xs text-muted-foreground truncate">webPort：{wp.join(', ')}</div> : null; })()}
                                    </div>
                                ))}
                                {containers.length === 0 ? <div className="text-sm text-muted-foreground p-2">暂无容器</div> : null}
                            </div>
                        </ScrollArea>
                    </div>

                    <div className="aix-list-section">
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">ExploitIdea 状态</div>
                            <Badge>{exploitIdeas.length}</Badge>
                        </div>
                        <ScrollArea className="mt-2 h-[220px] rounded-lg border border-border bg-background/20 min-w-0">
                            <div className="p-2 space-y-2 min-w-0">
                                {exploitIdeas.map((e, idx) => (
                                    <div
                                        key={e?.exploitIdeaId ?? e?.ExploitIdeaId ?? idx}
                                        className="rounded-lg border border-border bg-background/10 px-3 py-2 hover:bg-muted/30 cursor-pointer min-w-0 overflow-hidden"
                                        onClick={() => openDetail('exploitIdea', String(e?.exploitIdeaId ?? e?.ExploitIdeaId ?? 'exploitIdea'), e)}
                                    >
                                        <div className="flex items-center justify-between gap-2 min-w-0">
                                            <div className="font-semibold text-sm truncate min-w-0">{String(e?.exploitIdeaId ?? e?.ExploitIdeaId ?? 'exploitIdea')}</div>
                                            <Badge className="shrink-0" variant={badgeVariantForExploitState(e?.state ?? e?.State)}>{String(e?.state ?? e?.State ?? '-')}</Badge>
                                        </div>
                                        {(e?.review_reason || e?.ReviewReason) && (String(e?.state ?? e?.State ?? '')).includes('审核失败') && (
                                            <div className="mt-1 text-xs text-destructive break-words" style={{ overflowWrap: 'anywhere' }}>审核失败原因：{String(e?.review_reason ?? e?.ReviewReason)}</div>
                                        )}
                                    </div>
                                ))}
                                {exploitIdeas.length === 0 ? <div className="text-sm text-muted-foreground p-2">暂无 exploitIdea</div> : null}
                            </div>
                        </ScrollArea>
                    </div>

                    <div className="aix-list-section">
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">ExploitChain 状态</div>
                            <Badge>{exploitChains.length}</Badge>
                        </div>
                        <ScrollArea className="mt-2 h-[220px] rounded-lg border border-border bg-background/20 min-w-0">
                            <div className="p-2 space-y-2 min-w-0">
                                {exploitChains.map((e, idx) => (
                                    <div
                                        key={e?.exploit_chain_id ?? e?.ExploitChainId ?? idx}
                                        className="rounded-lg border border-border bg-background/10 px-3 py-2 hover:bg-muted/30 cursor-pointer min-w-0 overflow-hidden"
                                        onClick={() => openDetail('exploitChain', String(e?.exploit_chain_id ?? e?.ExploitChainId ?? 'exploitChain'), e)}
                                    >
                                        <div className="flex items-center justify-between gap-2 min-w-0">
                                            <div className="font-semibold text-sm truncate min-w-0">{String(e?.exploit_chain_id ?? e?.ExploitChainId ?? 'exploitChain')}</div>
                                            <Badge className="shrink-0" variant={badgeVariantForExploitState(e?.state ?? e?.State)}>{String(e?.state ?? e?.State ?? '-')}</Badge>
                                        </div>
                                        {(e?.review_reason || e?.ReviewReason) && (String(e?.state ?? e?.State ?? '')).includes('审核失败') && (
                                            <div className="mt-1 text-xs text-destructive break-words" style={{ overflowWrap: 'anywhere' }}>审核失败原因：{String(e?.review_reason ?? e?.ReviewReason)}</div>
                                        )}
                                    </div>
                                ))}
                                {exploitChains.length === 0 ? <div className="text-sm text-muted-foreground p-2">暂无 exploitChain</div> : null}
                            </div>
                        </ScrollArea>
                    </div>
                </div>

                <div className="grid gap-4 min-w-0 overflow-hidden">
                    <div>
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">环境信息</div>
                            <Badge variant={envInfo ? 'success' : 'warning'}>{envInfo ? 'ready' : 'pending'}</Badge>
                        </div>
                        <div className="mt-2 rounded-lg border border-border bg-background/20 p-3">
                            {envInfo ? (
                                <div style={{ paddingTop: 6 }}>
                                    <div style={{ marginTop: 10 }}>
                                        <div style={{ fontWeight: 800, letterSpacing: 0.2, marginBottom: 8 }}>基础信息</div>
                                        <FieldRow label="containerId" value={get(envInfo, ['containerId', 'ContainerId'])} />
                                    </div>

                                    <div style={{ marginTop: 14 }}>
                                        <div style={{ fontWeight: 800, letterSpacing: 0.2, marginBottom: 8 }}>登录信息</div>
                                        <FieldRow label="username" value={get(get(envInfo, ['loginInfo']), ['username'])} />
                                        <FieldRow label="password" value={get(get(envInfo, ['loginInfo']), ['password'])} />
                                        <FieldRow label="loginURL" value={get(get(envInfo, ['loginInfo']), ['loginURL'])} />
                                        <FieldRow label="credentials" value={get(get(envInfo, ['loginInfo']), ['credentials'])} />
                                    </div>

                                    <div style={{ marginTop: 14 }}>
                                        <div style={{ fontWeight: 800, letterSpacing: 0.2, marginBottom: 8 }}>数据库信息</div>
                                        <FieldRow label="username" value={get(get(envInfo, ['dbInfo']), ['username'])} />
                                        <FieldRow label="password" value={get(get(envInfo, ['dbInfo']), ['password'])} />
                                        <FieldRow label="Host" value={get(get(envInfo, ['dbInfo']), ['Host', 'host'])} />
                                        <FieldRow label="Base" value={get(get(envInfo, ['dbInfo']), ['Base', 'base'])} />
                                    </div>

                                    <div style={{ marginTop: 14 }}>
                                        <div style={{ fontWeight: 800, letterSpacing: 0.2, marginBottom: 8 }}>路由示例</div>
                                        {Array.isArray(get(envInfo, ['routeInfo'])) && (get(envInfo, ['routeInfo']) as any[]).length > 0 ? (
                                            <ScrollArea className="h-[160px] rounded-lg border border-border bg-background/10">
                                                <div className="p-2 space-y-2">
                                                    {(get(envInfo, ['routeInfo']) as any[]).slice(0, 50).map((r, idx) => (
                                                        <div key={idx} className="text-xs text-foreground/90">{String(r)}</div>
                                                    ))}
                                                </div>
                                            </ScrollArea>
                                        ) : (
                                            <div className="text-sm text-muted-foreground">暂无路由示例</div>
                                        )}
                                    </div>
                                </div>
                            ) : (
                                <div className="text-sm text-muted-foreground">暂无环境信息（需 Ops Agent 完成环境搭建后才会生成）</div>
                            )}
                        </div>
                    </div>

                    <div>
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">报告</div>
                            <Badge>{Object.keys(reports || {}).length}</Badge>
                        </div>
                        <div className="mt-2 flex flex-wrap gap-2">
                            <Button
                                size="sm"
                                variant="outline"
                                disabled={!detailProject}
                                onClick={() => {
                                    if (!detailProject) return;
                                    const name = encodeURIComponent(detailProject);
                                    downloadFileWithAuth(`${baseURL}/projects/${name}/reports/downloadAll`, `${detailProject}-report.zip`);
                                }}
                            >
                                下载全部
                            </Button>
                        </div>
                        <ScrollArea className="mt-2 h-[240px] rounded-lg border border-border bg-background/20">
                            <div className="p-2 space-y-2">
                                {Object.keys(reports || {}).map((id) => {
                                    const filename = String(reports[id] ?? id);
                                    return (
                                        <div key={id} className="rounded-lg border border-border bg-background/10 px-3 py-2">
                                            <div className="flex items-center justify-between gap-2">
                                                <div className="font-semibold text-sm break-all">{filename}</div>
                                                <div className="flex shrink-0 items-center gap-2">
                                                    <Button size="sm" variant="secondary" onClick={async () => {
                                                        if (!detailProject) return;
                                                        try {
                                                            const content = await fetchReportText(detailProject, id);
                                                            openReportPreview(id, filename, content);
                                                        } catch (e: any) {
                                                            openDetail('json', 'Report Preview Error', { id, filename, error: String(e?.message ?? e ?? 'error') });
                                                        }
                                                    }}>预览</Button>
                                                    <Button size="sm" variant="outline" onClick={() => {
                                                        if (!detailProject) return;
                                                        const name = encodeURIComponent(detailProject);
                                                        const rid = encodeURIComponent(id);
                                                        downloadFileWithAuth(`${baseURL}/projects/${name}/reports/download/${rid}`, filename);
                                                    }}>下载</Button>
                                                </div>
                                            </div>
                                            <div className="mt-1 text-xs text-muted-foreground">id：{id}</div>
                                        </div>
                                    );
                                })}
                                {Object.keys(reports || {}).length === 0 ? <div className="text-sm text-muted-foreground p-2">暂无报告</div> : null}
                            </div>
                        </ScrollArea>
                    </div>

                    <div>
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold">决策大脑 · 上下文占用</div>
                            <Badge variant={contextBreakdown.max_context > 0 && contextBreakdown.total > contextBreakdown.max_context * 0.85 ? 'destructive' : 'secondary'}>
                                {contextBreakdown.total > 0 ? `${contextBreakdown.total.toLocaleString()} / ${contextBreakdown.max_context.toLocaleString()} tokens` : '等待数据'}
                            </Badge>
                        </div>
                        <div className="mt-2 rounded-lg border border-border bg-background/20 p-3">
                            {contextBreakdown.sections.length === 0 ? (
                                <div className="text-sm text-muted-foreground">决策大脑尚未启动，暂无上下文数据</div>
                            ) : (() => {
                                const sectionLabels: Record<string, string> = {
                                    system_prompt: '系统提示词',
                                    task_goal: '任务目标',
                                    agent_runtime: 'Agent 运行状态',
                                    exploit_idea_list: 'ExploitIdea 列表',
                                    exploit_chain_list: 'ExploitChain 列表',
                                    env: '环境信息',
                                    containers: '容器信息',
                                    key_info: '关键信息',
                                    conversation_history: '对话历史',
                                };
                                const sectionColors: Record<string, string> = {
                                    system_prompt: '#6366f1',
                                    task_goal: '#8b5cf6',
                                    agent_runtime: '#f59e0b',
                                    exploit_idea_list: '#10b981',
                                    exploit_chain_list: '#06b6d4',
                                    env: '#3b82f6',
                                    containers: '#64748b',
                                    key_info: '#a855f7',
                                    conversation_history: '#ef4444',
                                };
                                const total = contextBreakdown.total || 1;
                                const maxCtx = contextBreakdown.max_context || total;
                                const usagePct = Math.min(100, (total / maxCtx) * 100);
                                const sorted = [...contextBreakdown.sections].sort((a, b) => b.tokens - a.tokens);
                                return (
                                    <div className="space-y-3">
                                        {/* Usage bar */}
                                        <div>
                                            <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                                                <span>上下文使用率</span>
                                                <span style={{ color: usagePct > 85 ? '#ef4444' : usagePct > 60 ? '#f59e0b' : '#10b981' }}>{usagePct.toFixed(1)}%</span>
                                            </div>
                                            <div style={{ height: 8, borderRadius: 4, background: 'rgba(255,255,255,0.06)', overflow: 'hidden' }}>
                                                <div style={{
                                                    height: '100%', borderRadius: 4, width: `${usagePct}%`,
                                                    background: usagePct > 85 ? '#ef4444' : usagePct > 60 ? '#f59e0b' : '#10b981',
                                                    transition: 'width 0.5s ease',
                                                }} />
                                            </div>
                                        </div>
                                        {/* Stacked bar */}
                                        <div>
                                            <div className="text-xs text-muted-foreground mb-1">各部分占比</div>
                                            <div style={{ display: 'flex', height: 24, borderRadius: 6, overflow: 'hidden', background: 'rgba(255,255,255,0.04)' }}>
                                                {sorted.filter(s => s.tokens > 0).map((s) => {
                                                    const pct = (s.tokens / total) * 100;
                                                    if (pct < 0.5) return null;
                                                    return (
                                                        <div
                                                            key={s.name}
                                                            title={`${sectionLabels[s.name] || s.name}: ${s.tokens.toLocaleString()} tokens (${pct.toFixed(1)}%)`}
                                                            style={{
                                                                width: `${pct}%`, minWidth: pct > 2 ? 2 : 0,
                                                                background: sectionColors[s.name] || '#94a3b8',
                                                                transition: 'width 0.5s ease',
                                                                borderRight: '1px solid rgba(0,0,0,0.2)',
                                                            }}
                                                        />
                                                    );
                                                })}
                                            </div>
                                        </div>
                                        {/* Legend + details */}
                                        <div className="space-y-1.5">
                                            {sorted.map((s) => {
                                                const pct = (s.tokens / total) * 100;
                                                return (
                                                    <div key={s.name} className="flex items-center gap-2 text-xs">
                                                        <div style={{ width: 10, height: 10, borderRadius: 2, background: sectionColors[s.name] || '#94a3b8', flexShrink: 0 }} />
                                                        <div className="flex-1 text-foreground/80 truncate">{sectionLabels[s.name] || s.name}</div>
                                                        <div className="text-muted-foreground tabular-nums">{s.tokens.toLocaleString()}</div>
                                                        <div className="text-muted-foreground tabular-nums w-[48px] text-right">{pct.toFixed(1)}%</div>
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    </div>
                                );
                            })()}
                        </div>
                    </div>
                </div>
            </div>

            {/* Team Chat */}
            <div className="mt-4">
                <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                        <div className="text-sm font-semibold">团队聊天</div>
                        <button
                            className="text-xs px-2 py-0.5 rounded border border-border bg-muted/40 hover:bg-primary/20 text-muted-foreground hover:text-foreground transition-colors"
                            onClick={() => { setChatFullscreen(true); setTimeout(() => chatEndRefFull.current?.scrollIntoView({ behavior: 'auto' }), 50); }}
                            title="展开聊天"
                        >⤢ 展开</button>
                    </div>
                    <div className="flex items-center gap-1 flex-wrap">
                        {(() => {
                            const allNames: string[] = [];
                            Object.values(digitalHumanRoster || {}).forEach((list: any) => {
                                if (Array.isArray(list)) list.forEach((h: any) => {
                                    const n = String(h?.persona_name ?? '');
                                    if (n && n !== '-' && !allNames.includes(n)) allNames.push(n);
                                });
                            });
                            return (
                                <>
                                    <button
                                        className="text-xs px-2 py-0.5 rounded-full border border-border bg-muted/40 hover:bg-primary/20 text-muted-foreground hover:text-foreground transition-colors"
                                        onClick={() => setChatInput(prev => {
                                            const trimmed = prev.replace(/^@\S*\s?/, '').trim();
                                            return '';
                                        })}
                                    >
                                        决策大脑
                                    </button>
                                    <button
                                        className="text-xs px-2 py-0.5 rounded-full border border-border bg-muted/40 hover:bg-primary/20 text-muted-foreground hover:text-foreground transition-colors"
                                        onClick={() => setChatInput('@all ')}
                                    >
                                        @全体
                                    </button>
                                    {allNames.map(n => (
                                        <button
                                            key={n}
                                            className="text-xs px-2 py-0.5 rounded-full border border-border bg-muted/40 hover:bg-primary/20 text-muted-foreground hover:text-foreground transition-colors"
                                            onClick={() => setChatInput(`@${n} `)}
                                        >
                                            @{n}
                                        </button>
                                    ))}
                                </>
                            );
                        })()}
                    </div>
                </div>
                <div className="mt-2 wx-inline-chat flex flex-col overflow-hidden" style={{ height: 300 }}>
                    <ScrollArea className="flex-1 px-3 py-2.5">
                        <div className="space-y-3">
                            {chatMessages.length === 0 ? (
                                <div className="text-[11px] text-white/20 py-6 text-center">发送消息与数字人或决策大脑对话</div>
                            ) : null}
                            {chatMessages.map((m, idx) => {
                                const avatarSrc = (() => {
                                    if (m.role === 'user') return null;
                                    const file = m.avatar_file && m.avatar_file !== '' ? m.avatar_file : 'system.png';
                                    try { return new URL(`./assets/avatar/${file}`, import.meta.url).toString(); }
                                    catch { return new URL(`./assets/avatar/system.png`, import.meta.url).toString(); }
                                })();
                                const isSelf = m.role === 'user';
                                return (
                                <div key={idx} className={`flex ${isSelf ? 'flex-row-reverse' : 'flex-row'} items-start gap-2`}>
                                    {!isSelf && avatarSrc ? (
                                        <img src={avatarSrc} alt={m.persona_name || ''} className="wx-avatar shrink-0" style={{ width: 30, height: 30 }} />
                                    ) : isSelf ? (
                                        <div className="wx-avatar shrink-0 bg-[#57a85c] flex items-center justify-center text-white text-[10px] font-bold" style={{ width: 30, height: 30 }}>我</div>
                                    ) : null}
                                    <div className={`max-w-[85%] sm:max-w-[75%] min-w-0`} style={{ wordBreak: 'break-word', overflowWrap: 'break-word' }}>
                                        {!isSelf && m.persona_name && (
                                            <div className="text-[10px] font-medium text-white/25 mb-0.5 ml-1 truncate">{m.persona_name}</div>
                                        )}
                                        <div className={`wx-inline-bubble ${isSelf ? 'wx-inline-bubble--self' : 'wx-inline-bubble--other'}`}>
                                            <div className="prose prose-invert prose-sm max-w-none break-words [&>*:first-child]:mt-0 [&>*:last-child]:mb-0 [&_pre]:whitespace-pre-wrap [&_pre]:break-all [&_code]:break-all" style={{ wordBreak: 'break-all', overflowWrap: 'anywhere' }}><ReactMarkdown>{m.text}</ReactMarkdown></div>
                                        </div>
                                        <div className={`text-[9px] mt-0.5 ${isSelf ? 'text-right mr-1' : 'ml-1'} text-white/15`}>{m.ts}</div>
                                    </div>
                                </div>
                                );
                            })}
                            <div ref={chatEndRef} />
                        </div>
                    </ScrollArea>
                    <div className="wx-input-bar px-3 py-2 flex items-center gap-2">
                        <Input
                            className="wx-input flex-1 text-[13px] h-9"
                            placeholder="输入消息… @姓名 私聊 · @all 群发"
                            value={chatInput}
                            onChange={e => setChatInput(e.target.value)}
                            onKeyDown={e => {
                                if (e.key === 'Enter' && !e.shiftKey) {
                                    e.preventDefault();
                                    sendChat();
                                }
                            }}
                            disabled={chatSending}
                        />
                        <Button size="sm" disabled={chatSending || !chatInput.trim()} onClick={sendChat} className="wx-send-btn h-9 px-4 text-[13px]">
                            {chatSending ? '…' : '发送'}
                        </Button>
                    </div>
                </div>
            </div>

        </CardContent>
    </Card>
                </>
                )}
            </div>
        </div>

        {/* Fullscreen Chat — rendered at top level, completely independent of the project page */}
        {chatFullscreen && (() => {
            const getRoleLabel = (toolName: string) => {
                if (toolName.includes('Agent-Ops-OpsEnvScoutAgent')) return '远程环境运维工程师';
                if (toolName.includes('Agent-Ops-OpsCommonAgent')) return '环境运维专家';
                if (toolName.includes('Agent-Analyze-')) return '代码审计专家';
                if (toolName.includes('Agent-Verifier-')) return '漏洞验证专家';
                if (toolName.includes('Agent-Report-')) return '报告编写专员';
                return toolName;
            };
            const allDHNames: { name: string; avatar: string; state: string }[] = [];
            const dhProfileMap: Record<string, { persona_name: string; gender: string; personality: string; age: number; role: string; avatar_file: string; state: string; agent_id: string; digital_human_id: string }> = {};
            Object.entries(digitalHumanRoster || {}).forEach(([toolName, list]) => {
                if (Array.isArray(list)) list.forEach((h: any) => {
                    const n = String(h?.persona_name ?? '');
                    const av = String(h?.avatar_file ?? '');
                    const st = String(h?.state ?? 'idle');
                    if (n && n !== '-' && !allDHNames.some(x => x.name === n)) {
                        allDHNames.push({ name: n, avatar: av, state: st });
                        dhProfileMap[n] = {
                            persona_name: n,
                            gender: String(h?.gender ?? '-'),
                            personality: String(h?.personality ?? '-'),
                            age: Number(h?.age ?? 0),
                            role: getRoleLabel(toolName),
                            avatar_file: av,
                            state: st,
                            agent_id: String(h?.agent_id ?? ''),
                            digital_human_id: String(h?.digital_human_id ?? ''),
                        };
                    }
                });
            });
            const getAvatarUrl = (file: string) => {
                const f = file && file !== '-' ? file : 'system.png';
                try { return new URL(`./assets/avatar/${f}`, import.meta.url).toString(); }
                catch { return new URL(`./assets/avatar/system.png`, import.meta.url).toString(); }
            };
            const profileCardContent = (p: typeof dhProfileMap[string], avatarUrl: string) => (
                <div className="rounded-xl border border-border bg-card shadow-lg p-3 w-[220px] max-w-[90vw] text-card-foreground">
                    <div className="flex items-center gap-2.5 mb-2">
                        <img src={avatarUrl} alt={p.persona_name} className="h-10 w-10 rounded-full border border-border object-cover shrink-0" />
                        <div className="min-w-0">
                            <div className="text-sm font-bold truncate">{p.persona_name}</div>
                        </div>
                    </div>
                    <div className="space-y-1 text-[11px]">
                        <div className="flex justify-between"><span className="text-muted-foreground">性格</span><span className="text-right max-w-[140px] truncate">{p.personality}</span></div>
                        <div className="flex justify-between"><span className="text-muted-foreground">状态</span><span>{p.state === 'busy' ? '🟡 忙碌' : '🟢 空闲'}</span></div>
                    </div>
                </div>
            );
            const fixedHoverHandler = (e: { currentTarget: HTMLDivElement }, show: boolean) => {
                const target = e.currentTarget;
                const card = target.querySelector('[data-profile-card]') as HTMLElement | null;
                if (!card) return;
                if (show) {
                    const rect = target.getBoundingClientRect();
                    card.style.position = 'fixed';
                    card.style.left = `${rect.right + 8}px`;
                    card.style.top = `${rect.top}px`;
                    card.style.display = 'block';
                } else {
                    card.style.display = 'none';
                }
            };
            return (
            <div className="fixed inset-0 z-[100] flex h-screen w-screen overflow-hidden wx-chat-bg" style={{ animation: 'fadeIn 0.15s ease-out' }}>
                {/* Left Sidebar — Contacts (WeChat style) */}
                <div className="aix-chat-sidebar w-[260px] shrink-0 border-r border-white/[0.04] flex flex-col wx-sidebar min-h-0">
                    <div className="px-4 py-3.5 border-b border-white/[0.04] shrink-0">
                        <div className="flex items-center justify-between">
                            <div className="text-[13px] font-bold tracking-wide text-white/80">通讯录</div>
                            <span className="text-[11px] text-white/30 font-medium">{allDHNames.length + 1} 人</span>
                        </div>
                    </div>
                    <ScrollArea className="flex-1 min-h-0">
                        <div className="p-2 space-y-0.5">
                            <button className="wx-sidebar-item w-full flex items-center gap-3 px-3 py-2.5 text-left" onClick={() => setChatInput('')}>
                                <img src={getAvatarUrl('system.png')} alt="决策大脑" className="wx-avatar wx-avatar-sm shrink-0" />
                                <div className="min-w-0 flex-1">
                                    <div className="text-[13px] font-medium truncate text-white/90">决策大脑</div>
                                    <div className="text-[11px] text-white/30 mt-0.5">团队指挥</div>
                                </div>
                                <span className="h-2 w-2 rounded-full bg-[#57a85c] shrink-0" />
                            </button>
                            <button className="wx-sidebar-item w-full flex items-center gap-3 px-3 py-2.5 text-left" onClick={() => setChatInput('@all ')}>
                                <div className="wx-avatar wx-avatar-sm shrink-0 bg-[#2a2d35] flex items-center justify-center text-[11px] font-bold text-white/50">All</div>
                                <div className="min-w-0 flex-1">
                                    <div className="text-[13px] font-medium truncate text-white/90">全体广播</div>
                                    <div className="text-[11px] text-white/30 mt-0.5">发送给所有人</div>
                                </div>
                            </button>
                            {allDHNames.length > 0 && <div className="border-t border-white/[0.04] my-1.5 mx-3" />}
                            {allDHNames.map(dh => (
                                <button key={dh.name} className="wx-sidebar-item w-full flex items-center gap-3 px-3 py-2.5 text-left" onClick={() => setChatInput(`@${dh.name} `)}>
                                    <div
                                        className="relative shrink-0"
                                        onMouseEnter={(e) => fixedHoverHandler(e, true)}
                                        onMouseLeave={(e) => fixedHoverHandler(e, false)}
                                    >
                                        <img src={getAvatarUrl(dh.avatar)} alt={dh.name} className="wx-avatar wx-avatar-sm" />
                                        {dhProfileMap[dh.name] && (
                                            <div data-profile-card="" className="z-[300] pointer-events-none" style={{ display: 'none' }}>
                                                {profileCardContent(dhProfileMap[dh.name], getAvatarUrl(dh.avatar))}
                                            </div>
                                        )}
                                    </div>
                                    <div className="min-w-0 flex-1">
                                        <div className="text-[13px] font-medium truncate text-white/90">{dh.name}</div>
                                        <div className="text-[11px] text-white/30 mt-0.5">{dh.state === 'busy' ? '忙碌中' : '空闲'}</div>
                                    </div>
                                    <span className={`h-2 w-2 rounded-full shrink-0 ${dh.state === 'busy' ? 'bg-amber-400' : 'bg-[#57a85c]'}`} />
                                </button>
                            ))}
                        </div>
                    </ScrollArea>
                    <div className="px-3 py-2.5 border-t border-white/[0.04] shrink-0">
                        <button className="w-full text-[12px] text-white/40 hover:text-white/70 transition-colors py-1.5" onClick={() => setChatFullscreen(false)}>← 返回项目</button>
                    </div>
                </div>

                {/* Right Main — Chat Area (WeChat style) */}
                <div className="aix-chat-main flex-1 flex flex-col min-w-0 min-h-0">
                    <div className="px-4 sm:px-5 py-3 border-b border-white/[0.04] shrink-0 flex items-center justify-between gap-2" style={{ background: '#1e2028' }}>
                        <div className="flex items-center gap-2 sm:gap-3 min-w-0">
                            <div className="text-[14px] sm:text-[15px] font-bold text-white/90 whitespace-nowrap">团队聊天</div>
                            <span className="text-[11px] text-white/25 font-medium">{chatMessages.length} 条消息</span>
                        </div>
                        <div className="text-[11px] text-white/25 hidden sm:block">@姓名 私聊 · @all 群发 · 无@ 发给决策大脑 · Esc 返回</div>
                        <button className="sm:hidden text-[12px] text-white/40 hover:text-white/70 shrink-0" onClick={() => setChatFullscreen(false)}>← 返回</button>
                    </div>
                    <ScrollArea className="flex-1 min-h-0">
                        <div className="px-4 sm:px-8 py-5 space-y-4 max-w-4xl mx-auto">
                            {chatMessages.length === 0 ? (
                                <div className="flex flex-col items-center justify-center py-24 text-white/20">
                                    <div className="text-5xl mb-4 opacity-40">💬</div>
                                    <div className="text-sm font-medium">暂无消息</div>
                                    <div className="text-xs mt-1.5 text-white/15">从左侧选择联系人开始对话</div>
                                </div>
                            ) : null}
                            {chatMessages.map((m, idx) => {
                                const avatarSrc = (() => {
                                    if (m.role === 'user') return null;
                                    const file = m.avatar_file && m.avatar_file !== '' ? m.avatar_file : 'system.png';
                                    try { return new URL(`./assets/avatar/${file}`, import.meta.url).toString(); }
                                    catch { return new URL(`./assets/avatar/system.png`, import.meta.url).toString(); }
                                })();
                                const isSelf = m.role === 'user';
                                return (
                                    <div key={idx} className={`flex ${isSelf ? 'flex-row-reverse' : 'flex-row'} items-start gap-2.5`}>
                                        {!isSelf && avatarSrc ? (
                                            <div
                                                className="relative shrink-0"
                                                onMouseEnter={(e) => fixedHoverHandler(e, true)}
                                                onMouseLeave={(e) => fixedHoverHandler(e, false)}
                                            >
                                                <img src={avatarSrc} alt={m.persona_name || ''} className="wx-avatar wx-avatar-md cursor-pointer" />
                                                {m.persona_name && dhProfileMap[m.persona_name] && (
                                                    <div data-profile-card="" className="z-[300] pointer-events-none" style={{ display: 'none' }}>
                                                        {profileCardContent(dhProfileMap[m.persona_name], avatarSrc)}
                                                    </div>
                                                )}
                                            </div>
                                        ) : isSelf ? (
                                            <div className="wx-avatar wx-avatar-md shrink-0 bg-[#57a85c] flex items-center justify-center text-white text-xs font-bold">我</div>
                                        ) : null}
                                        <div className={`max-w-[80%] sm:max-w-[60%] min-w-0`} style={{ wordBreak: 'break-word', overflowWrap: 'break-word' }}>
                                            {!isSelf && m.persona_name && (
                                                <div className={`text-[11px] font-medium text-white/30 mb-1 ${isSelf ? 'text-right mr-2' : 'ml-2'}`}>{m.persona_name}</div>
                                            )}
                                            <div className={`wx-bubble ${isSelf ? 'wx-bubble--self' : 'wx-bubble--other'}`}>
                                                <div className="prose prose-invert prose-sm max-w-none break-words [&>*:first-child]:mt-0 [&>*:last-child]:mb-0 [&_pre]:whitespace-pre-wrap [&_pre]:break-all [&_code]:break-all" style={{ wordBreak: 'break-all', overflowWrap: 'anywhere' }}><ReactMarkdown>{m.text}</ReactMarkdown></div>
                                            </div>
                                            <div className={`text-[10px] mt-1 ${isSelf ? 'text-right mr-2' : 'ml-2'} text-white/20`}>{m.ts}</div>
                                        </div>
                                    </div>
                                );
                            })}
                            <div ref={chatEndRefFull} />
                        </div>
                    </ScrollArea>
                    <div className="wx-input-bar px-4 sm:px-5 py-3 shrink-0">
                        <div className="max-w-4xl mx-auto flex items-center gap-2.5">
                            <Input
                                className="wx-input flex-1 h-10"
                                placeholder="输入消息…"
                                value={chatInput}
                                onChange={e => setChatInput(e.target.value)}
                                onKeyDown={e => {
                                    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChat(); }
                                    if (e.key === 'Escape') { setChatFullscreen(false); }
                                }}
                                disabled={chatSending}
                                autoFocus
                            />
                            <Button disabled={chatSending || !chatInput.trim()} onClick={sendChat} className="wx-send-btn h-10 px-6">
                                {chatSending ? '发送中…' : '发送'}
                            </Button>
                        </div>
                    </div>
                </div>
            </div>
            );
        })()}
    </>
    );

    async function sendChat() {
        const msg = chatInput.trim();
        if (!msg || !detailProject || chatSending) return;
        const now = new Date().toLocaleTimeString();
        setChatMessages(prev => [...prev, { role: 'user', text: msg, ts: now }]);
        setChatInput('');
        setChatSending(true);
        try {
            const name = encodeURIComponent(detailProject);
            await apiPost<string>(`/projects/${name}/chat`, { message: msg });
        } catch (e: any) {
            setChatMessages(prev => [...prev, { role: 'system', text: `Error: ${String(e?.message ?? e)}`, ts: new Date().toLocaleTimeString() }]);
        } finally {
            setChatSending(false);
            setTimeout(() => {
                chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
                chatEndRefFull.current?.scrollIntoView({ behavior: 'smooth' });
            }, 100);
        }
    }
}

export default App;
