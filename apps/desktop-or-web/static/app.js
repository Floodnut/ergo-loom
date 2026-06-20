if (new URLSearchParams(window.location.search).get("desktop") === "1" || window.ergoLoom) {
    document.documentElement.classList.add("desktop-shell");
}
const state = {
    sessions: [],
    projects: [],
    routes: [],
    project: null,
    projectRoutes: [],
    profiles: [],
    models: [],
    usage: [],
    tools: [],
    authStatuses: [],
    contextUsage: null,
    selectedSessionId: null,
    selectedModelId: "",
    thinkingEffort: "medium",
    pendingApprovals: [],
    workspaceEvents: [],
    activeWorkspaceTab: "activity",
    terminalTabs: [
        {
            id: "terminal-1",
            title: "zsh 1",
            pendingCommand: "",
            workingDir: "",
            history: [],
            running: false,
            lines: [
                { text: "$ ergo status", className: "" },
                { text: "Local runtime ready", className: "muted" },
            ],
        },
    ],
    activeTerminalId: "terminal-1",
    fileTabs: [
        {
            id: "file-1",
            title: "No file",
            path: "",
            content: "No file open",
            status: "idle",
            size: 0,
        },
    ],
    recentFiles: [],
    activeFileId: "file-1",
    runningSessionId: "",
    searchTimer: null,
    showAllSessions: false,
};
const searchProviders = [
    {
        id: "local",
        label: "Local workspace",
        async search(query) {
            const projectMatches = state.projects
                .filter((project) => project.DisplayName.toLowerCase().includes(query.toLowerCase()))
                .map((project) => ({
                id: project.ID,
                type: "project",
                title: project.DisplayName,
                detail: project.RootPath || "local workspace",
            }));
            const data = await request(`/api/sessions/search?q=${encodeURIComponent(query)}`);
            const sessionMatches = (data.sessions || []).map((session) => ({
                id: session.ID,
                type: "chat",
                title: session.Title,
                detail: `${session.SourceTool} · ${formatDate(session.UpdatedAt)}`,
            }));
            return [...projectMatches, ...sessionMatches];
        },
    },
];
function q(selector) {
    const element = document.querySelector(selector);
    if (!element) {
        throw new Error(`Missing element: ${selector}`);
    }
    return element;
}
const els = {
    shell: q("#app-shell"),
    sessions: q("#session-list"),
    messages: q("#messages"),
    title: q("#chat-title"),
    subtitle: q("#chat-subtitle"),
    contextMeter: q("#context-meter"),
    providers: q("#providers"),
    agents: q("#agents"),
    routes: q("#routes"),
    projectRoutes: q("#project-routes"),
    projects: q("#project-list"),
    projectName: q("#project-name"),
    createProject: q("#create-project"),
    openSearch: q("#open-search"),
    searchModal: q("#search-modal"),
    searchBackdrop: q("#search-backdrop"),
    sessionSearch: q("#session-search"),
    searchResults: q("#search-results"),
    projectToggle: q("#project-toggle"),
    projectMenuButton: q("#project-menu-button"),
    projectMenu: q("#project-menu"),
    renameProject: q("#rename-project"),
    aiUsagePanel: q("#ai-usage-panel"),
    usageToggle: q("#usage-toggle"),
    usageGaugeFill: q("#usage-gauge-fill"),
    navUsage: q("#nav-token-usage"),
    routePicker: q("#route-picker"),
    addRoute: q("#add-route"),
    activeRoute: q("#active-route"),
    chatAIPanel: q("#chat-ai-panel"),
    chatMore: q("#chat-more"),
    usage: q("#usage"),
    navToggle: q("#nav-toggle"),
    reasoningPane: q("#reasoning-pane"),
    reasoningToggle: q("#reasoning-toggle"),
    reasoningStream: q("#reasoning-stream"),
    reasoningCount: q("#reasoning-count"),
    approvalPanel: q("#tool-approval-panel"),
    approvalList: q("#tool-approval-list"),
    newSession: q("#new-session"),
    composer: q("#composer"),
    input: q("#message-input"),
    sendButton: q("#send-message"),
    modelPicker: q("#model-picker"),
    modelPickerLabel: q("#model-picker-label"),
    modelMenu: q("#model-menu"),
    modelMenuList: q("#model-menu-list"),
    modelSearch: q("#model-search"),
    modelEffort: q("#model-effort"),
    terminalForm: q("#terminal-form"),
    terminalCommand: q("#terminal-command"),
    terminalOutput: q("#terminal-output"),
    terminalPending: q("#terminal-pending"),
    terminalPendingCommand: q("#terminal-pending-command"),
    terminalRun: q("#terminal-run"),
    terminalCancel: q("#terminal-cancel"),
    terminalCwd: q("#terminal-cwd"),
    terminalHistory: q("#terminal-history"),
    terminalStop: q("#terminal-stop"),
    workspaceTabs: q("#workspace-tabs"),
    workspaceActivity: q("#workspace-activity"),
    workspaceApprovals: q("#workspace-approvals"),
    authStatusList: q("#auth-status-list"),
    toolRegistryList: q("#tool-registry-list"),
    terminalTabs: q("#terminal-tabs"),
    newTerminalTab: q("#new-terminal-tab"),
    fileTabs: q("#file-tabs"),
    newFileTab: q("#new-file-tab"),
    fileOpenForm: q("#file-open-form"),
    filePath: q("#file-path"),
    fileRecent: q("#file-recent"),
    fileReload: q("#file-reload"),
    fileMeta: q("#file-meta"),
    fileContent: q("#file-content"),
};
const terminalControllers = new Map();
async function request(path, options = {}) {
    const response = await fetch(path, {
        headers: { "Content-Type": "application/json" },
        ...options,
    });
    if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(body.error || `Request failed: ${response.status}`);
    }
    return response.json();
}
async function loadState() {
    loadThinkingEffort();
    const data = await request("/api/state");
    state.sessions = data.sessions || [];
    state.projects = data.projects || [];
    state.routes = data.routes || [];
    state.project = data.project || null;
    state.projectRoutes = data.projectRoutes || [];
    state.profiles = data.profiles || [];
    state.models = data.models || [];
    state.usage = data.usage || [];
    state.tools = data.tools || [];
    state.authStatuses = data.auth || [];
    renderProjectName();
    renderProjects();
    renderSessions();
    renderProjectRoutes();
    renderRoutePicker();
    renderModelPicker();
    renderNavUsage();
    renderWorkspaceActivity();
    renderAuthStatuses();
    renderToolRegistrySummary();
    renderTerminalPanel();
    renderFilePanel();
    renderRegistry(els.providers, groupedProviderRegistry(data.providers || []));
    renderRegistry(els.agents, data.agents || []);
    renderRoutes(state.routes);
    renderUsage(state.usage);
    if (!state.selectedSessionId && state.sessions.length > 0) {
        await selectSession(state.sessions[0].ID);
    }
    else if (!state.selectedSessionId) {
        renderEmptyChat();
    }
}
function renderProjectName() {
    if (!els.projectName)
        return;
    els.projectName.textContent = state.project?.DisplayName || "Default Project";
}
async function addProjectRoute() {
    if (!state.project || !els.routePicker.value)
        return;
    await addRouteToCurrentProject(els.routePicker.value);
}
async function addRouteToCurrentProject(routeID) {
    if (!state.project || !routeID)
        return;
    await request(`/api/projects/${encodeURIComponent(state.project.ID)}/routes`, {
        method: "POST",
        body: JSON.stringify({ routeId: routeID }),
    });
    await loadState();
}
async function addProviderRoute(providerID) {
    const existing = state.projectRoutes.find((item) => item.Enabled && item.Route.ProviderPluginID === providerID);
    if (existing)
        return;
    const route = state.routes
        .filter((item) => item.ProviderPluginID === providerID)
        .sort((a, b) => routeStatusRank(a) - routeStatusRank(b))[0];
    if (!route)
        return;
    await addRouteToCurrentProject(route.ID);
}
function routeStatusRank(route) {
    if (route.Status === "available")
        return 0;
    if (route.SupportsHandoff)
        return 1;
    return 2;
}
async function createProject() {
    const displayName = window.prompt("Project name")?.trim() || "";
    if (!displayName)
        return;
    const data = await request("/api/projects", {
        method: "POST",
        body: JSON.stringify({ displayName }),
    });
    state.projects = [data.project, ...state.projects.filter((project) => project.ID !== data.project.ID)];
    renderProjects();
}
async function renameProject() {
    if (!state.project)
        return;
    const displayName = window.prompt("Project name", state.project.DisplayName)?.trim() || "";
    if (!displayName || displayName === state.project.DisplayName)
        return;
    const data = await request(`/api/projects/${encodeURIComponent(state.project.ID)}`, {
        method: "PATCH",
        body: JSON.stringify({ displayName }),
    });
    state.project = data.project;
    state.projects = state.projects.map((project) => project.ID === data.project.ID ? data.project : project);
    renderProjectName();
    renderProjects();
    els.projectMenu.hidden = true;
}
async function removeProjectRoute(routeId) {
    if (!state.project)
        return;
    await request(`/api/projects/${encodeURIComponent(state.project.ID)}/routes/${encodeURIComponent(routeId)}`, {
        method: "DELETE",
    });
    await loadState();
}
function toggleNavigation() {
    els.shell.classList.toggle("nav-collapsed");
}
function toggleProjectChats() {
    const collapsed = els.shell.classList.toggle("project-collapsed");
    els.projectToggle.setAttribute("aria-expanded", String(!collapsed));
}
function toggleProjectMenu() {
    els.projectMenu.hidden = !els.projectMenu.hidden;
}
function openSearchModal() {
    els.searchModal.hidden = false;
    els.sessionSearch.value = "";
    renderSearchResults(recentSearchItems());
    requestAnimationFrame(() => els.sessionSearch.focus());
}
function closeSearchModal() {
    els.searchModal.hidden = true;
    els.sessionSearch.value = "";
    renderSearchResults(recentSearchItems());
}
function toggleProjectRoutes() {
    const expanded = els.chatAIPanel.hidden;
    els.chatAIPanel.hidden = !expanded;
    els.activeRoute.setAttribute("aria-expanded", String(expanded));
}
function toggleAIUsage() {
    const collapsed = els.aiUsagePanel.classList.toggle("collapsed");
    els.usageToggle.setAttribute("aria-expanded", String(!collapsed));
}
function toggleReasoning() {
    const collapsed = els.reasoningPane.classList.toggle("collapsed");
    els.reasoningToggle.setAttribute("aria-expanded", String(!collapsed));
}
function handleComposerKeydown(event) {
    if (event.key !== "Enter" || event.shiftKey || event.isComposing) {
        return;
    }
    event.preventDefault();
    els.composer.requestSubmit();
}
async function createSession() {
    const data = await request("/api/sessions", {
        method: "POST",
        body: JSON.stringify({ title: "New chat" }),
    });
    state.selectedSessionId = data.session.ID;
    await loadState();
    await selectSession(data.session.ID);
    els.input.focus();
}
async function selectSession(sessionId, options = {}) {
    const shouldResetActivity = options.resetActivity ?? true;
    state.selectedSessionId = sessionId;
    renderSessions();
    if (shouldResetActivity) {
        resetReasoningStream();
        clearToolApprovals();
    }
    const data = await request(`/api/sessions/${encodeURIComponent(sessionId)}`);
    els.title.textContent = data.session.Title;
    els.subtitle.textContent = state.project?.DisplayName || "Default Project";
    state.contextUsage = data.context || null;
    renderContextMeter();
    renderMessages(data.messages || []);
    closeSearchModal();
}
async function sendMessage(event) {
    event.preventDefault();
    const content = els.input.value.trim();
    if (!content)
        return;
    if (!selectedModelOption()) {
        window.alert("Select an available model for this project first.");
        return;
    }
    if (!state.selectedSessionId) {
        await createSession();
    }
    els.input.value = "";
    setComposerBusy(true);
    state.runningSessionId = state.selectedSessionId;
    renderSessions();
    try {
        await streamMessage(state.selectedSessionId, content);
        await loadState();
        await selectSession(state.selectedSessionId, { resetActivity: false });
    }
    catch (error) {
        appendActivityEvent("error", { text: error.message || String(error), toolName: "chat" });
    }
    finally {
        state.runningSessionId = "";
        renderSessions();
        setComposerBusy(false);
    }
}
function setComposerBusy(busy) {
    if (!busy && state.runningSessionId) {
        state.runningSessionId = "";
        renderSessions();
    }
    els.input.disabled = busy;
    els.sendButton.disabled = busy || !selectedModelOption();
}
function queueTerminalCommand(event) {
    event.preventDefault();
    const command = els.terminalCommand.value.trim();
    if (!command)
        return;
    const tab = activeTerminalTab();
    tab.pendingCommand = command;
    renderTerminalPanel();
}
function cancelTerminalCommand() {
    const tab = activeTerminalTab();
    tab.pendingCommand = "";
    renderTerminalPanel();
}
async function runTerminalCommand() {
    const tab = activeTerminalTab();
    const command = tab.pendingCommand;
    if (!command)
        return;
    const controller = new AbortController();
    terminalControllers.set(tab.id, controller);
    tab.running = true;
    if (!tab.history.includes(command))
        tab.history.unshift(command);
    tab.history = tab.history.slice(0, 20);
    renderTerminalPanel();
    els.terminalRun.disabled = true;
    appendTerminalLine(`$ ${command}`);
    const runningLine = appendTerminalLine("running...", "muted");
    appendWorkspaceEvent("tool", { toolName: "zsh", command, text: tab.workingDir ? `cwd: ${tab.workingDir}` : "Queued from terminal tab" });
    try {
        const data = await request("/api/terminal/run", {
            method: "POST",
            signal: controller.signal,
            body: JSON.stringify({
                command,
                sessionId: state.selectedSessionId || "",
                workingDir: tab.workingDir || "",
            }),
        });
        runningLine.text = "completed";
        renderTerminalRun(data.run);
        appendWorkspaceEvent("result", {
            toolName: "zsh",
            command,
            text: [data.run.Stdout, data.run.Stderr].filter(Boolean).join("\n"),
            status: data.run.Status,
        });
        els.terminalCommand.value = "";
        cancelTerminalCommand();
    }
    catch (error) {
        runningLine.text = error.name === "AbortError" ? "cancelled by user" : error.message || String(error);
        runningLine.className = "error";
        appendWorkspaceEvent("error", { toolName: "zsh", command, text: error.message || String(error) });
        renderTerminalPanel();
    }
    finally {
        tab.running = false;
        terminalControllers.delete(tab.id);
        renderTerminalPanel();
        els.terminalRun.disabled = false;
    }
}
function stopTerminalCommand() {
    const tab = activeTerminalTab();
    const controller = terminalControllers.get(tab.id);
    if (controller)
        controller.abort();
}
async function streamMessage(sessionId, content) {
    const selected = selectedModelOption();
    const response = await fetch(`/api/sessions/${encodeURIComponent(sessionId)}/messages/stream`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            content,
            routeId: selected?.routeId || "",
            modelId: selected?.model.ID || "",
            thinkingEffort: state.thinkingEffort,
        }),
    });
    if (!response.ok || !response.body) {
        throw new Error(`Request failed: ${response.status}`);
    }
    let assistantNode = null;
    let assistantContent = "";
    let lastStatus = "";
    resetReasoningStream();
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
        const { done, value } = await reader.read();
        if (done)
            break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";
        for (const line of lines) {
            if (!line.trim())
                continue;
            const event = JSON.parse(line);
            if (event.type === "message") {
                appendMessage(event.payload.Role, event.payload.Content);
            }
            if (event.type === "assistant_start") {
                assistantNode = appendMessage("assistant", "");
            }
            if (event.type === "assistant_delta" && assistantNode) {
                assistantContent += event.payload.text;
                renderMarkdown(assistantNode.querySelector(".content"), assistantContent.trimEnd());
                els.messages.scrollTop = els.messages.scrollHeight;
            }
            if (event.type === "assistant_status" && event.payload.text !== lastStatus) {
                lastStatus = event.payload.text;
                appendReasoning(event.payload.text);
            }
            if (event.type === "tool_start") {
                appendActivityEvent("tool", event.payload);
            }
            if (event.type === "tool_result") {
                appendActivityEvent("result", event.payload);
            }
            if (event.type === "approval_request") {
                appendActivityEvent("approval", event.payload);
                addToolApproval(event.payload);
            }
            if (event.type === "tool_error" || event.type === "turn_aborted") {
                appendActivityEvent("error", event.payload);
            }
            if (event.type === "error") {
                appendActivityEvent("error", { text: event.payload.message, toolName: "chat" });
                return;
            }
        }
    }
}
function renderSessions() {
    els.sessions.replaceChildren();
    if (state.sessions.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta project-empty";
        empty.textContent = "No chats yet";
        els.sessions.append(empty);
        return;
    }
    const visibleSessions = state.showAllSessions ? state.sessions : state.sessions.slice(0, 5);
    for (const session of visibleSessions) {
        const row = document.createElement("div");
        row.className = `session-item${session.ID === state.selectedSessionId ? " active" : ""}${session.ID === state.runningSessionId ? " running" : ""}`;
        const button = document.createElement("button");
        button.className = "session-main";
        button.type = "button";
        button.title = `${session.Title} 열기`;
        button.addEventListener("click", () => selectSession(session.ID));
        const title = document.createElement("div");
        title.className = "session-title";
        title.textContent = session.Title;
        const meta = document.createElement("div");
        meta.className = "meta";
        meta.textContent = session.ID === state.runningSessionId ? "Running" : formatDate(session.UpdatedAt);
        button.append(title, meta);
        const rename = document.createElement("button");
        rename.className = "session-rename";
        rename.type = "button";
        rename.title = "채팅 이름 변경";
        rename.textContent = "…";
        rename.addEventListener("click", (event) => {
            event.stopPropagation();
            renameSession(session.ID);
        });
        row.append(button, rename);
        els.sessions.append(row);
    }
    if (!state.showAllSessions && state.sessions.length > visibleSessions.length) {
        const more = document.createElement("button");
        more.type = "button";
        more.className = "show-more-sessions";
        more.textContent = "More";
        more.addEventListener("click", () => {
            state.showAllSessions = true;
            renderSessions();
        });
        els.sessions.append(more);
    }
}
async function renameSession(sessionID) {
    const session = state.sessions.find((item) => item.ID === sessionID);
    if (!session)
        return;
    const title = window.prompt("채팅 이름", session.Title)?.trim();
    if (!title || title === session.Title)
        return;
    const data = await request(`/api/sessions/${encodeURIComponent(sessionID)}`, {
        method: "PATCH",
        body: JSON.stringify({ title }),
    });
    state.sessions = state.sessions.map((item) => item.ID === sessionID ? data.session : item);
    if (state.selectedSessionId === sessionID) {
        els.title.textContent = data.session.Title;
    }
    renderSessions();
}
function renderProjects() {
    els.projects.replaceChildren();
    if (state.projects.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta";
        empty.textContent = "No projects";
        els.projects.append(empty);
        return;
    }
    for (const project of state.projects.filter((item) => item.ID !== state.project?.ID)) {
        const row = document.createElement("button");
        row.type = "button";
        row.className = "project-list-item";
        row.innerHTML = `
      <span class="project-folder" aria-hidden="true"></span>
      <strong>${escapeHTML(project.DisplayName)}</strong>
      <span class="project-arrow">›</span>
    `;
        els.projects.append(row);
    }
}
function searchSessions() {
    window.clearTimeout(state.searchTimer);
    state.searchTimer = window.setTimeout(async () => {
        const query = els.sessionSearch.value.trim();
        els.searchResults.replaceChildren();
        if (!query)
            return;
        const results = [];
        for (const provider of searchProviders) {
            const matches = await provider.search(query);
            results.push(...matches.map((match) => ({ ...match, provider: provider.label })));
        }
        renderSearchResults(results);
    }, 160);
}
function renderSearchResults(results) {
    els.searchResults.replaceChildren();
    if (results.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta";
        empty.textContent = els.sessionSearch?.value ? "No matches" : "Recent chats and projects";
        els.searchResults.append(empty);
        return;
    }
    for (const item of results) {
        const row = document.createElement("button");
        row.type = "button";
        row.className = "search-result";
        row.innerHTML = `
      <strong>${escapeHTML(item.title)}</strong>
      <span>${escapeHTML(item.type)} · ${escapeHTML(item.detail)}</span>
    `;
        if (item.type === "chat") {
            row.addEventListener("click", () => selectSession(item.id));
        }
        else {
            row.addEventListener("click", closeSearchModal);
        }
        els.searchResults.append(row);
    }
}
function recentSearchItems() {
    const projects = state.projects.slice(0, 3).map((project) => ({
        id: project.ID,
        type: "project",
        title: project.DisplayName,
        detail: project.RootPath || "local workspace",
        provider: "Local workspace",
    }));
    const chats = state.sessions.slice(0, 8).map((session) => ({
        id: session.ID,
        type: "chat",
        title: session.Title,
        detail: `${session.SourceTool} · ${formatDate(session.UpdatedAt)}`,
        provider: "Local workspace",
    }));
    return [...projects, ...chats];
}
function renderMessages(messages) {
    els.messages.replaceChildren();
    if (messages.length === 0) {
        renderEmptyChat();
        return;
    }
    for (const message of messages) {
        appendMessage(message.Role, message.Content);
    }
    els.messages.scrollTop = els.messages.scrollHeight;
}
function appendMessage(roleName, text) {
    const item = document.createElement("article");
    item.className = `message ${roleName}`;
    const role = document.createElement("div");
    role.className = "role";
    role.textContent = roleName;
    const content = document.createElement("div");
    content.className = "content";
    renderMarkdown(content, text);
    item.append(role, content);
    els.messages.append(item);
    els.messages.scrollTop = els.messages.scrollHeight;
    return item;
}
function renderMarkdown(container, text) {
    container.replaceChildren();
    const blocks = markdownBlocks(String(text || ""));
    for (const block of blocks) {
        container.append(block);
    }
}
function markdownBlocks(text) {
    const lines = text.replace(/\r\n/g, "\n").split("\n");
    const nodes = [];
    let paragraph = [];
    let list = null;
    let code = null;
    let codeLang = "";
    const flushParagraph = () => {
        if (paragraph.length === 0)
            return;
        const p = document.createElement("p");
        appendInlineMarkdown(p, paragraph.join(" "));
        nodes.push(p);
        paragraph = [];
    };
    const flushList = () => {
        if (!list)
            return;
        nodes.push(list);
        list = null;
    };
    const flushCode = () => {
        if (!code)
            return;
        const pre = document.createElement("pre");
        const codeNode = document.createElement("code");
        if (codeLang)
            codeNode.dataset.lang = codeLang;
        codeNode.textContent = code.join("\n");
        pre.append(codeNode);
        nodes.push(pre);
        code = null;
        codeLang = "";
    };
    for (const line of lines) {
        const fence = line.match(/^```([\w-]*)\s*$/);
        if (fence) {
            if (code) {
                flushCode();
            }
            else {
                flushParagraph();
                flushList();
                code = [];
                codeLang = fence[1] || "";
            }
            continue;
        }
        if (code) {
            code.push(line);
            continue;
        }
        if (!line.trim()) {
            flushParagraph();
            flushList();
            continue;
        }
        const heading = line.match(/^(#{1,3})\s+(.+)$/);
        if (heading) {
            flushParagraph();
            flushList();
            const level = Math.min(3, heading[1].length + 2);
            const h = document.createElement(`h${level}`);
            appendInlineMarkdown(h, heading[2]);
            nodes.push(h);
            continue;
        }
        const listItem = line.match(/^\s*(?:[-*]|\d+\.)\s+(.+)$/);
        if (listItem) {
            flushParagraph();
            if (!list)
                list = document.createElement("ul");
            const li = document.createElement("li");
            appendInlineMarkdown(li, listItem[1]);
            list.append(li);
            continue;
        }
        paragraph.push(line.trim());
    }
    flushParagraph();
    flushList();
    flushCode();
    if (nodes.length === 0) {
        const p = document.createElement("p");
        p.textContent = "";
        nodes.push(p);
    }
    return nodes;
}
function appendInlineMarkdown(parent, text) {
    const pattern = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\((https?:\/\/[^)\s]+)\))/g;
    let index = 0;
    for (const match of text.matchAll(pattern)) {
        if (match.index > index) {
            parent.append(document.createTextNode(text.slice(index, match.index)));
        }
        const token = match[0];
        if (token.startsWith("**")) {
            const strong = document.createElement("strong");
            strong.textContent = token.slice(2, -2);
            parent.append(strong);
        }
        else if (token.startsWith("`")) {
            const code = document.createElement("code");
            code.textContent = token.slice(1, -1);
            parent.append(code);
        }
        else {
            const link = token.match(/^\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)$/);
            if (link) {
                const a = document.createElement("a");
                a.textContent = link[1];
                a.href = link[2];
                a.target = "_blank";
                a.rel = "noreferrer";
                parent.append(a);
            }
        }
        index = match.index + token.length;
    }
    if (index < text.length) {
        parent.append(document.createTextNode(text.slice(index)));
    }
}
function appendReasoning(text) {
    const item = document.createElement("div");
    item.className = "reasoning-item";
    item.textContent = text;
    els.reasoningStream.append(item);
    els.reasoningStream.scrollTop = els.reasoningStream.scrollHeight;
    els.reasoningCount.textContent = String(els.reasoningStream.children.length);
    return item;
}
function appendActivityEvent(kind, payload = {}) {
    appendWorkspaceEvent(kind, payload);
    mirrorToolEventToTerminal(kind, payload);
    const item = document.createElement("div");
    item.className = `reasoning-item activity-${kind}`;
    const title = document.createElement("div");
    title.className = "activity-title";
    title.textContent = activityTitle(kind, payload);
    item.append(title);
    const detailText = [payload.command, payload.text].filter(Boolean).join("\n");
    if (detailText) {
        const detail = document.createElement("pre");
        detail.className = "activity-detail";
        detail.textContent = detailText;
        item.append(detail);
    }
    if (kind === "approval") {
        const actions = document.createElement("div");
        actions.className = "activity-actions";
        if (payload.status === "declined") {
            actions.innerHTML = `<span class="activity-status">Declined</span>`;
        }
        else {
            actions.innerHTML = `<span class="activity-status">Waiting for approval above composer</span>`;
        }
        item.append(actions);
    }
    els.reasoningStream.append(item);
    els.reasoningStream.scrollTop = els.reasoningStream.scrollHeight;
    els.reasoningCount.textContent = String(els.reasoningStream.children.length);
    return item;
}
function mirrorToolEventToTerminal(kind, payload = {}) {
    const command = payload.command || "";
    const text = payload.text || "";
    const toolName = String(payload.toolName || payload.toolId || "");
    const looksLikeCommand = command || /command|shell|zsh|exec/i.test(toolName);
    if (!looksLikeCommand)
        return;
    if (kind === "tool") {
        appendTerminalLine(command ? `$ ${command}` : `tool started · ${toolName}`, "muted");
        switchWorkspaceTab("terminals");
    }
    else if (kind === "result") {
        if (text) {
            for (const line of text.trimEnd().split("\n").slice(-80)) {
                appendTerminalLine(line);
            }
        }
    }
    else if (kind === "error") {
        appendTerminalLine(text || `tool interrupted · ${toolName}`, "error");
    }
    else if (kind === "approval") {
        switchWorkspaceTab("activity");
    }
}
function appendWorkspaceEvent(kind, payload = {}) {
    state.workspaceEvents.unshift({
        id: `event-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        kind,
        payload,
        createdAt: new Date(),
    });
    state.workspaceEvents = state.workspaceEvents.slice(0, 80);
    renderWorkspaceActivity();
}
function renderAuthStatuses() {
    els.authStatusList.replaceChildren();
    if (!state.authStatuses.length)
        return;
    for (const item of state.authStatuses) {
        const row = document.createElement("div");
        row.className = `auth-status-row ${item.connected ? "connected" : "missing"}`;
        row.innerHTML = `
      <div>
        <strong>${escapeHTML(item.label)}</strong>
        <span>${escapeHTML(item.detail || item.status || "")}</span>
      </div>
      <span>${item.connected ? "Ready" : "Needs setup"}</span>
    `;
        els.authStatusList.append(row);
    }
}
function renderToolRegistrySummary() {
    els.toolRegistryList.replaceChildren();
    if (!state.tools.length)
        return;
    const label = document.createElement("div");
    label.className = "workspace-panel-label";
    label.textContent = "Tool Registry";
    const list = document.createElement("div");
    list.className = "tool-registry-grid";
    for (const tool of state.tools) {
        const item = document.createElement("span");
        item.className = tool.Enabled ? "tool-registry-chip" : "tool-registry-chip disabled";
        item.textContent = tool.DisplayName;
        item.title = `${tool.ID} · ${tool.Kind}`;
        list.append(item);
    }
    els.toolRegistryList.append(label, list);
}
function renderWorkspaceActivity() {
    els.workspaceActivity.replaceChildren();
    if (state.workspaceEvents.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta";
        empty.textContent = "No tool activity yet";
        els.workspaceActivity.append(empty);
    }
    for (const event of state.workspaceEvents) {
        const item = document.createElement("div");
        item.className = `workspace-activity-item activity-${event.kind}`;
        const title = document.createElement("div");
        title.className = "activity-title";
        title.textContent = activityTitle(event.kind, event.payload);
        const meta = document.createElement("div");
        meta.className = "meta";
        meta.textContent = event.createdAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
        item.append(title, meta);
        const detailText = [event.payload.command, event.payload.text].filter(Boolean).join("\n").trim();
        if (detailText) {
            const detail = document.createElement("pre");
            detail.className = "activity-detail";
            detail.textContent = detailText;
            item.append(detail);
        }
        els.workspaceActivity.append(item);
    }
    renderWorkspaceApprovals();
}
function renderWorkspaceApprovals() {
    els.workspaceApprovals.replaceChildren();
    if (state.pendingApprovals.length === 0)
        return;
    const label = document.createElement("div");
    label.className = "workspace-panel-label";
    label.textContent = "Pending approvals";
    els.workspaceApprovals.append(label);
    for (const approval of state.pendingApprovals) {
        els.workspaceApprovals.append(toolApprovalCard(approval));
    }
}
function addToolApproval(payload) {
    if (!payload.approvalId)
        return;
    state.pendingApprovals = state.pendingApprovals.filter((item) => item.approvalId !== payload.approvalId);
    state.pendingApprovals.push({
        ...payload,
        directInput: "",
        suggestions: approvalSuggestions(payload),
    });
    renderToolApprovals();
}
function renderToolApprovals() {
    els.approvalList.replaceChildren();
    els.approvalPanel.hidden = state.pendingApprovals.length === 0;
    for (const approval of state.pendingApprovals) {
        els.approvalList.append(toolApprovalCard(approval));
    }
    renderWorkspaceApprovals();
}
function toolApprovalCard(approval) {
    const card = document.createElement("section");
    card.className = "tool-approval-card";
    const title = document.createElement("div");
    title.className = "tool-approval-title";
    title.textContent = "도구 실행 승인 필요";
    const approve = approvalLineButton(1, "승인", approval.command || approval.text || "요청된 동작 실행", "accept", approval);
    const deny = approvalLineButton(2, "거절", "실행하지 않음", "decline", approval);
    const direct = document.createElement("label");
    direct.className = "tool-approval-direct";
    direct.innerHTML = `<span class="tool-approval-marker">✎</span><strong>직접입력:</strong>`;
    const input = document.createElement("input");
    input.type = "text";
    input.placeholder = "명령 또는 요청 내용을 입력";
    input.value = approval.directInput || "";
    input.addEventListener("input", () => {
        approval.directInput = input.value;
    });
    const directButton = approvalActionButton("적용", "decline", approval, input);
    direct.append(input, directButton);
    const suggestions = document.createElement("div");
    suggestions.className = "tool-approval-suggestions";
    approval.suggestions.forEach((suggestion, index) => {
        suggestions.append(approvalLineButton(index + 3, `제안${index + 1}`, suggestion, "decline", approval));
    });
    card.append(title, approve, deny, suggestions, direct);
    return card;
}
function approvalLineButton(index, label, value, decision, payload) {
    const button = document.createElement("button");
    button.className = "tool-approval-line";
    button.type = "button";
    button.innerHTML = `
    <span class="tool-approval-marker">${index}</span>
    <strong>${escapeHTML(label)}:</strong>
    <span>${escapeHTML(value)}</span>
  `;
    button.addEventListener("click", () => resolveApproval(payload, decision, button));
    return button;
}
function approvalActionButton(label, decision, payload, input) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = label;
    button.addEventListener("click", () => {
        payload.directInput = input.value;
        resolveApproval(payload, decision, button);
    });
    return button;
}
async function resolveApproval(payload, decision, sourceButton) {
    if (!payload.approvalId)
        return;
    const card = sourceButton.closest(".tool-approval-card");
    for (const item of card.querySelectorAll("button, input"))
        item.disabled = true;
    try {
        await request(`/api/tool-approvals/${encodeURIComponent(payload.approvalId)}`, {
            method: "POST",
            body: JSON.stringify({
                decision,
                directInput: payload.directInput || "",
            }),
        });
        state.pendingApprovals = state.pendingApprovals.filter((item) => item.approvalId !== payload.approvalId);
        appendWorkspaceEvent(decision === "accept" ? "approval" : "error", {
            ...payload,
            status: decision,
            text: decision === "accept" ? "Approved by user" : "Declined by user",
        });
        renderToolApprovals();
    }
    catch (error) {
        const status = document.createElement("span");
        status.className = "activity-status error";
        status.textContent = error.message || "Approval failed";
        card.append(status);
    }
}
function approvalSuggestions(payload) {
    const raw = payload.raw || {};
    const fromPayload = Array.isArray(raw.suggestions) ? raw.suggestions.filter(Boolean) : [];
    if (fromPayload.length > 0)
        return fromPayload;
    return [
        "명령을 실행하지 않고 필요한 이유만 설명",
        "실행할 명령을 채팅에만 제안",
    ].filter(Boolean);
}
function activityTitle(kind, payload) {
    const tool = payload.toolName || "tool";
    if (kind === "error")
        return `Interrupted · ${tool}`;
    if (kind === "approval" && payload.status === "declined")
        return `Approval declined · ${tool}`;
    if (kind === "approval")
        return `Approval required · ${tool}${payload.sessionId ? ` · current chat` : ""}`;
    if (kind === "result")
        return `Tool result · ${tool}`;
    return `Tool call · ${tool}`;
}
function resetReasoningStream() {
    els.reasoningStream.replaceChildren();
    els.reasoningCount.textContent = "0";
}
function clearToolApprovals() {
    state.pendingApprovals = [];
    renderToolApprovals();
}
function renderEmptyChat() {
    els.title.textContent = "New chat";
    els.subtitle.textContent = state.project?.DisplayName || "Default Project";
    state.contextUsage = null;
    renderContextMeter();
    els.messages.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.innerHTML = `
    <img src="/icon.svg" alt="" aria-hidden="true">
    <span>Start a local chat</span>
  `;
    els.messages.append(empty);
}
function renderContextMeter() {
    const usage = state.contextUsage || {};
    const messageCount = usage.MessageCount || 0;
    const estimatedTokens = usage.EstimatedTokens || 0;
    const promptTokens = usage.PromptTokens || 0;
    const completionTokens = usage.CompletionTokens || 0;
    const providerChats = usage.ProviderChats || 0;
    const ledgerTokens = promptTokens + completionTokens;
    const tokenLabel = ledgerTokens > 0
        ? `${ledgerTokens.toLocaleString()} used · ${estimatedTokens.toLocaleString()} ctx est.`
        : `${estimatedTokens.toLocaleString()} ctx est.`;
    els.contextMeter.textContent = `${messageCount} messages · ${tokenLabel} · ${providerChats} provider chats`;
}
function activeTerminalTab() {
    let tab = state.terminalTabs.find((item) => item.id === state.activeTerminalId);
    if (!tab) {
        tab = state.terminalTabs[0];
        state.activeTerminalId = tab.id;
    }
    return tab;
}
function activeFileTab() {
    let tab = state.fileTabs.find((item) => item.id === state.activeFileId);
    if (!tab) {
        tab = state.fileTabs[0];
        state.activeFileId = tab.id;
    }
    return tab;
}
function switchWorkspaceTab(tabID) {
    state.activeWorkspaceTab = tabID;
    for (const button of els.workspaceTabs.querySelectorAll("[data-workspace-tab]")) {
        const tabButton = button;
        tabButton.classList.toggle("active", tabButton.dataset.workspaceTab === tabID);
    }
    for (const panel of document.querySelectorAll("[data-workspace-panel]")) {
        const workspacePanel = panel;
        const active = workspacePanel.dataset.workspacePanel === tabID;
        workspacePanel.classList.toggle("active", active);
        workspacePanel.hidden = !active;
    }
}
function renderTerminalPanel() {
    const active = activeTerminalTab();
    els.terminalTabs.replaceChildren();
    for (const tab of state.terminalTabs) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = tab.id === state.activeTerminalId ? "active" : "";
        button.innerHTML = `<span>${escapeHTML(tab.title)}</span><span class="tab-close" aria-hidden="true">×</span>`;
        button.addEventListener("click", () => {
            state.activeTerminalId = tab.id;
            renderTerminalPanel();
        });
        button.querySelector(".tab-close").addEventListener("click", (event) => {
            event.stopPropagation();
            closeTerminalTab(tab.id);
        });
        els.terminalTabs.append(button);
    }
    els.terminalOutput.replaceChildren();
    for (const line of active.lines) {
        const node = document.createElement("div");
        node.className = `terminal-line ${line.className || ""}`.trim();
        node.textContent = line.text;
        els.terminalOutput.append(node);
    }
    els.terminalPending.hidden = !active.pendingCommand;
    els.terminalPendingCommand.textContent = active.pendingCommand || "";
    els.terminalCwd.value = active.workingDir || "";
    els.terminalStop.disabled = !active.running;
    els.terminalHistory.replaceChildren();
    const historyPlaceholder = document.createElement("option");
    historyPlaceholder.value = "";
    historyPlaceholder.textContent = "History";
    els.terminalHistory.append(historyPlaceholder);
    for (const command of active.history || []) {
        const option = document.createElement("option");
        option.value = command;
        option.textContent = command;
        els.terminalHistory.append(option);
    }
    els.terminalOutput.parentElement.scrollTop = els.terminalOutput.parentElement.scrollHeight;
}
function newTerminalTab() {
    const next = state.terminalTabs.length + 1;
    const tab = {
        id: `terminal-${Date.now()}`,
        title: `zsh ${next}`,
        pendingCommand: "",
        workingDir: activeTerminalTab()?.workingDir || "",
        history: [],
        running: false,
        lines: [{ text: "$ ergo status", className: "" }, { text: "Local runtime ready", className: "muted" }],
    };
    state.terminalTabs.push(tab);
    state.activeTerminalId = tab.id;
    switchWorkspaceTab("terminals");
    renderTerminalPanel();
}
function closeTerminalTab(tabId) {
    if (state.terminalTabs.length === 1) {
        const tab = activeTerminalTab();
        tab.pendingCommand = "";
        tab.running = false;
        tab.lines = [{ text: "$ ergo status", className: "" }, { text: "Local runtime ready", className: "muted" }];
        terminalControllers.get(tab.id)?.abort();
        terminalControllers.delete(tab.id);
        renderTerminalPanel();
        return;
    }
    terminalControllers.get(tabId)?.abort();
    terminalControllers.delete(tabId);
    state.terminalTabs = state.terminalTabs.filter((tab) => tab.id !== tabId);
    if (state.activeTerminalId === tabId) {
        state.activeTerminalId = state.terminalTabs[0].id;
    }
    renderTerminalPanel();
}
function appendTerminalLine(text, className = "") {
    const tab = activeTerminalTab();
    const line = { text, className };
    tab.lines.push(line);
    renderTerminalPanel();
    return line;
}
function renderTerminalRun(run) {
    const output = [run.Stdout, run.Stderr].filter(Boolean).join("\n").trimEnd();
    if (output) {
        for (const line of output.split("\n")) {
            appendTerminalLine(line);
        }
    }
    appendTerminalLine(`exit ${run.ExitCode} · ${run.Status}`, run.ExitCode === 0 ? "muted" : "error");
}
function renderFilePanel() {
    const active = activeFileTab();
    els.fileTabs.replaceChildren();
    for (const tab of state.fileTabs) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = tab.id === state.activeFileId ? "active" : "";
        button.innerHTML = `<span>${escapeHTML(tab.title)}</span><span class="tab-close" aria-hidden="true">×</span>`;
        button.addEventListener("click", () => {
            state.activeFileId = tab.id;
            renderFilePanel();
        });
        button.querySelector(".tab-close").addEventListener("click", (event) => {
            event.stopPropagation();
            closeFileTab(tab.id);
        });
        els.fileTabs.append(button);
    }
    els.filePath.value = active.path || "";
    els.fileContent.textContent = active.content || "No file open";
    els.fileContent.className = active.status === "error" ? "error" : "";
    els.fileMeta.textContent = active.path
        ? `${active.path}${active.size ? ` · ${active.size.toLocaleString()} bytes` : ""}`
        : "No file selected";
    els.fileRecent.replaceChildren();
    const recentPlaceholder = document.createElement("option");
    recentPlaceholder.value = "";
    recentPlaceholder.textContent = "Recent";
    els.fileRecent.append(recentPlaceholder);
    for (const filePath of state.recentFiles) {
        const option = document.createElement("option");
        option.value = filePath;
        option.textContent = filePath.split("/").filter(Boolean).pop() || filePath;
        els.fileRecent.append(option);
    }
}
function newFileTab() {
    const next = state.fileTabs.length + 1;
    const tab = {
        id: `file-${Date.now()}`,
        title: `File ${next}`,
        path: "",
        content: "No file open",
        status: "idle",
        size: 0,
    };
    state.fileTabs.push(tab);
    state.activeFileId = tab.id;
    switchWorkspaceTab("files");
    renderFilePanel();
}
function closeFileTab(tabId) {
    if (state.fileTabs.length === 1) {
        const tab = activeFileTab();
        tab.title = "No file";
        tab.path = "";
        tab.content = "No file open";
        tab.status = "idle";
        tab.size = 0;
        renderFilePanel();
        return;
    }
    state.fileTabs = state.fileTabs.filter((tab) => tab.id !== tabId);
    if (state.activeFileId === tabId) {
        state.activeFileId = state.fileTabs[0].id;
    }
    renderFilePanel();
}
async function openFileInActiveTab(event) {
    event.preventDefault();
    await openFilePath(els.filePath.value.trim());
}
async function openFilePath(filePath) {
    const tab = activeFileTab();
    if (!filePath)
        return;
    tab.path = filePath;
    tab.title = filePath.split("/").filter(Boolean).pop() || filePath;
    tab.content = "Loading...";
    tab.status = "loading";
    renderFilePanel();
    try {
        const data = await request("/api/files/read", {
            method: "POST",
            body: JSON.stringify({ path: filePath }),
        });
        tab.path = data.path || filePath;
        tab.title = tab.path.split("/").filter(Boolean).pop() || tab.path;
        tab.content = data.content || "";
        tab.status = "loaded";
        tab.size = data.size || 0;
        state.recentFiles = [tab.path, ...state.recentFiles.filter((item) => item !== tab.path)].slice(0, 12);
        appendWorkspaceEvent("tool", { toolName: "file.read", command: tab.path, text: "Opened in file viewer" });
    }
    catch (error) {
        tab.content = error.message || String(error);
        tab.status = "error";
        tab.size = 0;
        appendWorkspaceEvent("error", { toolName: "file.read", command: filePath, text: tab.content });
    }
    renderFilePanel();
}
function reloadActiveFile() {
    const tab = activeFileTab();
    if (!tab.path)
        return;
    openFilePath(tab.path);
}
function renderRegistry(target, items) {
    target.replaceChildren();
    const list = document.createElement("div");
    list.className = "registry-list";
    for (const item of items) {
        const row = document.createElement("div");
        row.className = "registry-item";
        row.innerHTML = `<strong>${escapeHTML(item.DisplayName)}</strong><div class="meta">${escapeHTML(item.ID)}</div><span class="pill">${escapeHTML(item.Kind)}</span>`;
        list.append(row);
    }
    target.append(list);
}
function groupedProviderRegistry(items) {
    const groups = new Map();
    for (const item of items) {
        const groupID = providerGroupID(item.ID);
        const current = groups.get(groupID);
        if (current) {
            current.Enabled = current.Enabled || item.Enabled;
            continue;
        }
        groups.set(groupID, {
            ...item,
            ID: groupID === "codex-openai" ? "codex/openai" : item.ID,
            DisplayName: providerGroupLabel(groupID),
        });
    }
    return [...groups.values()].sort((a, b) => providerGroupOrder(a.ID) - providerGroupOrder(b.ID));
}
function renderUsage(items) {
    els.usage.replaceChildren();
    const providerRows = usageRowsForProviders(allProviderIDsForUsage(items));
    renderAIUsageList(els.usage, providerRows);
}
function renderNavUsage() {
    els.navUsage.replaceChildren();
    if (state.projectRoutes.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta";
        empty.textContent = "No AI selected";
        els.navUsage.append(empty);
        els.usageGaugeFill.style.width = "0%";
        return;
    }
    const providerRoutes = uniqueProjectProviderRoutes();
    const totalUsed = providerRoutes.reduce((sum, item) => sum + usageTotalForProvider(item.Route.ProviderPluginID), 0);
    const softCap = Math.max(50000, providerRoutes.length * 50000);
    els.usageGaugeFill.style.width = `${Math.min(100, Math.round((totalUsed / softCap) * 100))}%`;
    renderAIUsageList(els.navUsage, usageRowsForProviders(providerRoutes.map((item) => item.Route.ProviderPluginID)));
}
function renderAIUsageList(target, rows) {
    target.replaceChildren();
    if (rows.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta";
        empty.textContent = "No token usage recorded";
        target.append(empty);
        return;
    }
    const list = document.createElement("div");
    list.className = "ai-usage-list";
    for (const item of rows) {
        const row = document.createElement("div");
        row.className = "ai-usage-row";
        row.innerHTML = `
      <div class="ai-usage-head">
        <strong>${escapeHTML(item.providerName)}</strong>
        <span class="ai-usage-orb" style="--orb-color: ${escapeHTML(item.color)}"></span>
      </div>
      <div class="ai-usage-body">
        <span>${escapeHTML(item.accountLabel)}</span>
        <div class="ai-usage-gauge" aria-label="${escapeHTML(item.providerName)} usage">
          <span style="width: ${item.percent}%"></span>
        </div>
      </div>
      <div class="ai-usage-remaining">
        <span>${escapeHTML(item.remainingLabel)}</span>
        ${item.connected ? "" : `<button class="ai-connect-button" type="button" data-provider-id="${escapeHTML(item.providerID)}">Connect</button>`}
      </div>
    `;
        list.append(row);
    }
    target.append(list);
    for (const button of target.querySelectorAll(".ai-connect-button")) {
        button.addEventListener("click", () => connectProvider(button.dataset.providerId));
    }
}
function usageRowsForProviders(providerIDs) {
    const groupIDs = uniqueProviderGroups(providerIDs);
    return groupIDs.map((groupID) => {
        const providerIDsInGroup = providerIDsForGroup(groupID);
        const route = providerIDsInGroup.map(routeForProvider).find(Boolean) || null;
        const used = providerIDsInGroup.reduce((sum, providerID) => sum + usageTotalForProvider(providerID), 0);
        const providerCap = 50000;
        const profiles = providerIDsInGroup.map(profileForProvider).filter(Boolean);
        return {
            providerID: providerIDsInGroup[0],
            providerName: providerGroupLabel(groupID),
            connected: profiles.length > 0,
            accountLabel: profiles.length > 0 ? profiles.map((profile) => profile.DisplayName).join(" / ") : "account not connected",
            remainingLabel: remainingUsageLabel(route, used, providerCap),
            percent: Math.min(100, Math.round((used / providerCap) * 100)),
            color: providerColor(providerIDsInGroup[0]),
        };
    });
}
async function connectProvider(providerID) {
    const label = providerLabel(providerID);
    try {
        await request("/api/provider-profiles/connect", {
            method: "POST",
            body: JSON.stringify({ providerPluginId: providerID }),
        });
        await addProviderRoute(providerID);
        await loadState();
    }
    catch (error) {
        const message = error?.message || `${label} account bridge is not available`;
        const manualName = window.prompt(`${label} 자동 연결 실패\n${message}\n\n수동으로 표시할 계정명을 입력하면 프로필만 저장합니다.`, `${label} account`)?.trim() || "";
        if (!manualName)
            return;
        await request("/api/provider-profiles/connect", {
            method: "POST",
            body: JSON.stringify({ providerPluginId: providerID, displayName: manualName }),
        });
        await addProviderRoute(providerID);
        await loadState();
    }
}
function allProviderIDsForUsage(items) {
    const ids = new Set(state.projectRoutes.filter((item) => item.Enabled).map((item) => item.Route.ProviderPluginID));
    for (const item of items)
        ids.add(item.ProviderPluginID);
    return [...ids];
}
function usageTotalForProvider(providerID) {
    return state.usage
        .filter((item) => item.ProviderPluginID === providerID)
        .reduce((sum, item) => sum + item.PromptTokens + item.CompletionTokens, 0);
}
function routeForProvider(providerID) {
    return state.projectRoutes.find((item) => item.Route.ProviderPluginID === providerID)?.Route
        || state.routes.find((route) => route.ProviderPluginID === providerID)
        || null;
}
function profileForProvider(providerID) {
    return state.profiles.find((profile) => profile.ProviderPluginID === providerID && profile.IsDefault)
        || state.profiles.find((profile) => profile.ProviderPluginID === providerID)
        || null;
}
function remainingUsageLabel(route, used, cap) {
    const remaining = Math.max(0, cap - used);
    const method = route ? limitLabel(route.CostModel) : "tracked quota";
    return `${remaining.toLocaleString()} remaining · ${method}`;
}
function providerColor(providerID) {
    const colors = {
        anthropic: "#9b5e3f",
        codex: "#47685d",
        copilot: "#5b6f9f",
        cursor: "#5c5c58",
        gemini: "#c08a45",
        local: "#6f7b5a",
        openai: "#283f36",
    };
    return colors[providerID] || "#746f65";
}
function renderModelPicker() {
    const options = modelOptions();
    const selectable = options.filter(isModelSelectable);
    if (selectable.length === 0) {
        state.selectedModelId = "";
        els.modelPickerLabel.textContent = "No model";
        els.modelPicker.disabled = options.length === 0;
        els.sendButton.disabled = true;
        renderActiveRouteSummary();
        renderModelMenu(options);
        return;
    }
    const savedModelID = savedSelectedModelID();
    const current = selectable.find((item) => item.model.ID === state.selectedModelId)
        || selectable.find((item) => item.model.ID === savedModelID);
    const fallback = current || selectable.find((item) => item.model.IsDefault) || selectable[0];
    state.selectedModelId = fallback.model.ID;
    els.modelPickerLabel.textContent = modelDisplayName(fallback.model);
    renderThinkingEffort();
    els.modelPicker.disabled = false;
    els.sendButton.disabled = false;
    renderActiveRouteSummary();
    renderModelMenu(options);
}
function renderModelMenu(options) {
    els.modelMenuList.replaceChildren();
    const query = els.modelSearch.value.trim().toLowerCase();
    const filtered = options
        .filter((item) => {
        const haystack = `${modelDisplayName(item.model)} ${modelDescription(item.model)} ${providerLabel(item.model.ProviderPluginID)}`.toLowerCase();
        return haystack.includes(query);
    })
        .sort(compareModelOptions);
    if (filtered.length === 0) {
        const empty = document.createElement("div");
        empty.className = "model-menu-empty";
        empty.textContent = "No models";
        els.modelMenuList.append(empty);
        return;
    }
    let previousProvider = "";
    for (const item of filtered) {
        const provider = providerLabel(item.model.ProviderPluginID);
        if (provider !== previousProvider) {
            previousProvider = provider;
            const header = document.createElement("div");
            header.className = "model-menu-provider";
            header.textContent = provider;
            els.modelMenuList.append(header);
        }
        const row = document.createElement("button");
        row.type = "button";
        const selectable = isModelSelectable(item);
        const badge = modelBadge(item);
        const actionable = badge === "Add" || badge === "Connect";
        row.className = `model-menu-item${item.model.ID === state.selectedModelId ? " active" : ""}${selectable ? "" : " locked"}`;
        row.disabled = !selectable && !actionable;
        row.innerHTML = `
      <span class="model-check">${item.model.ID === state.selectedModelId ? "✓" : ""}</span>
      <span>
        <strong>${escapeHTML(modelDisplayName(item.model))}</strong>
        <small>${escapeHTML(modelDescription(item.model))}</small>
      </span>
      <em>${escapeHTML(badge)}</em>
    `;
        if (selectable) {
            row.addEventListener("click", () => selectModel(item.model.ID));
        }
        else if (badge === "Add") {
            row.addEventListener("click", () => addProviderRoute(item.model.ProviderPluginID));
        }
        else if (badge === "Connect") {
            row.addEventListener("click", () => connectProvider(item.model.ProviderPluginID));
        }
        els.modelMenuList.append(row);
    }
}
function toggleModelMenu() {
    const expanded = els.modelMenu.hidden;
    els.modelMenu.hidden = !expanded;
    els.modelPicker.setAttribute("aria-expanded", String(expanded));
    if (expanded) {
        els.modelSearch.value = "";
        renderModelMenu(modelOptions());
        requestAnimationFrame(() => els.modelSearch.focus());
    }
}
function selectModel(modelID) {
    state.selectedModelId = modelID;
    saveSelectedModelID(modelID);
    const selected = selectedModelOption();
    els.modelPickerLabel.textContent = selected ? modelDisplayName(selected.model) : "No model";
    renderActiveRouteSummary();
    els.modelMenu.hidden = true;
    els.modelPicker.setAttribute("aria-expanded", "false");
    renderModelMenu(modelOptions());
}
function modelDisplayName(model) {
    return model.DisplayName.replace(/^GPT-/, "GPT-");
}
function modelDescription(model) {
    const provider = providerLabel(model.ProviderPluginID);
    if (model.ProviderPluginID === "codex")
        return `${provider} · local subscription route`;
    if (model.ProviderPluginID === "anthropic")
        return `${provider} · web handoff / account route`;
    if (model.ProviderPluginID === "copilot")
        return `${provider} · SDK or VS Code bridge`;
    if (model.ProviderPluginID === "gemini")
        return `${provider} · CLI or web handoff`;
    if (model.ProviderPluginID === "openai")
        return `${provider} · web handoff`;
    return `${provider} model`;
}
function modelOptions() {
    const enabledProviders = new Map();
    for (const item of state.projectRoutes) {
        if (item.Enabled) {
            enabledProviders.set(item.Route.ProviderPluginID, item.Route);
        }
    }
    return state.models
        .map((model) => ({
        model,
        route: enabledProviders.get(model.ProviderPluginID) || null,
        routeId: enabledProviders.get(model.ProviderPluginID)?.ID || "",
        connected: Boolean(profileForProvider(model.ProviderPluginID)),
    }));
}
function compareModelOptions(a, b) {
    const groupCompare = providerGroupOrder(a.model.ProviderPluginID) - providerGroupOrder(b.model.ProviderPluginID);
    if (groupCompare !== 0)
        return groupCompare;
    const providerCompare = providerOrder(a.model.ProviderPluginID) - providerOrder(b.model.ProviderPluginID);
    if (providerCompare !== 0)
        return providerCompare;
    if (a.model.IsDefault !== b.model.IsDefault)
        return a.model.IsDefault ? -1 : 1;
    return modelDisplayName(a.model).localeCompare(modelDisplayName(b.model));
}
function selectedModelOption() {
    const options = modelOptions().filter(isModelSelectable);
    return options.find((item) => item.model.ID === state.selectedModelId) || options[0] || null;
}
function selectedModelStorageKey() {
    return `ergo-loom:selected-model:${state.project?.ID || "default"}`;
}
function savedSelectedModelID() {
    return window.localStorage.getItem(selectedModelStorageKey()) || "";
}
function saveSelectedModelID(modelID) {
    if (!modelID)
        return;
    window.localStorage.setItem(selectedModelStorageKey(), modelID);
}
function isModelSelectable(item) {
    if (!item.routeId || item.route?.Status !== "available")
        return false;
    if (item.model.Status === "available")
        return true;
    if (item.model.Status === "handoff" || item.model.Status === "bridge_required")
        return item.connected;
    return false;
}
function modelBadge(item) {
    if (!item.routeId)
        return "Add";
    if (item.route?.Status !== "available")
        return "Planned";
    if (item.model.Status === "available")
        return "";
    if (item.model.Status === "handoff")
        return item.connected ? "Handoff" : "Connect";
    if (item.model.Status === "bridge_required")
        return item.connected ? "Bridge" : "Connect";
    if (item.model.Status === "upgrade")
        return "Upgrade";
    return "Planned";
}
function renderThinkingEffort() {
    const label = thinkingEffortLabel(state.thinkingEffort);
    document.querySelectorAll(".model-picker-effort").forEach((node) => {
        node.textContent = label;
    });
    els.modelEffort.textContent = `Thinking effort · ${label}`;
}
function toggleThinkingEffort() {
    const levels = ["low", "medium", "high", "very_high"];
    const index = levels.indexOf(state.thinkingEffort);
    state.thinkingEffort = levels[(index + 1) % levels.length];
    window.localStorage.setItem("ergo-loom:thinking-effort", state.thinkingEffort);
    renderThinkingEffort();
}
function loadThinkingEffort() {
    const saved = window.localStorage.getItem("ergo-loom:thinking-effort") || "medium";
    state.thinkingEffort = ["low", "medium", "high", "very_high"].includes(saved) ? saved : "medium";
    renderThinkingEffort();
}
function thinkingEffortLabel(effort) {
    const labels = {
        low: "Low",
        medium: "Medium",
        high: "High",
        very_high: "Very high",
    };
    return labels[effort] || "Medium";
}
function uniqueProjectProviderRoutes() {
    const seen = new Set();
    const rows = [];
    for (const item of state.projectRoutes) {
        const providerID = item.Route.ProviderPluginID;
        if (!item.Enabled || seen.has(providerID))
            continue;
        seen.add(providerID);
        rows.push(item);
    }
    return rows;
}
function providerLabel(providerID) {
    const names = {
        anthropic: "Claude",
        codex: "Codex/ChatGPT",
        copilot: "VSCode Copilot",
        cursor: "Cursor",
        gemini: "Gemini",
        local: "Local Model",
        openai: "Codex/ChatGPT",
    };
    return names[providerID] || providerID;
}
function providerGroupID(providerID) {
    if (providerID === "codex" || providerID === "openai" || providerID === "codex/openai" || providerID === "codex-openai")
        return "codex-openai";
    return providerID;
}
function providerGroupLabel(groupID) {
    if (groupID === "codex-openai")
        return "Codex/ChatGPT";
    return providerLabel(groupID);
}
function providerIDsForGroup(groupID) {
    if (groupID === "codex-openai")
        return ["codex", "openai"];
    return [groupID];
}
function uniqueProviderGroups(providerIDs) {
    const seen = new Set();
    const groups = [];
    for (const providerID of providerIDs) {
        const groupID = providerGroupID(providerID);
        if (seen.has(groupID))
            continue;
        seen.add(groupID);
        groups.push(groupID);
    }
    return groups;
}
function providerGroupOrder(providerID) {
    const order = {
        "codex-openai": 10,
        anthropic: 20,
        copilot: 30,
        cursor: 40,
        gemini: 50,
        local: 60,
    };
    return order[providerGroupID(providerID)] || 999;
}
function providerOrder(providerID) {
    const order = {
        codex: 10,
        openai: 20,
        anthropic: 30,
        copilot: 40,
        cursor: 50,
        gemini: 60,
        local: 70,
    };
    return order[providerID] || 999;
}
function renderProjectRoutes() {
    els.projectRoutes.replaceChildren();
    renderActiveRouteSummary();
    if (state.projectRoutes.length === 0) {
        const empty = document.createElement("div");
        empty.className = "meta";
        empty.textContent = "Choose AI routes for this project";
        els.projectRoutes.append(empty);
        return;
    }
    const list = document.createElement("div");
    list.className = "registry-list compact";
    for (const item of state.projectRoutes) {
        const row = document.createElement("div");
        row.className = "route-row";
        const detail = document.createElement("div");
        detail.innerHTML = `<strong>${escapeHTML(providerLabel(item.Route.ProviderPluginID))}</strong><div class="meta">${escapeHTML(item.Route.DisplayName)} · ${escapeHTML(item.Route.AccessMode)} · ${escapeHTML(item.Route.CostModel)}</div>`;
        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "small-button";
        remove.textContent = "Remove";
        remove.addEventListener("click", () => removeProjectRoute(item.Route.ID));
        row.append(detail, remove);
        list.append(row);
    }
    els.projectRoutes.append(list);
}
function renderActiveRouteSummary() {
    const selected = selectedModelOption();
    if (selected) {
        els.activeRoute.textContent = `${providerLabel(selected.model.ProviderPluginID)} · ${modelDisplayName(selected.model)} ⌄`;
        return;
    }
    const firstRoute = state.projectRoutes.find((item) => item.Enabled);
    els.activeRoute.textContent = firstRoute ? `${firstRoute.Route.DisplayName} ⌄` : "No AI selected ⌄";
}
function renderRoutePicker() {
    els.routePicker.replaceChildren();
    const selected = new Set(state.projectRoutes.map((item) => item.Route.ID));
    const available = state.routes.filter((route) => !selected.has(route.ID));
    for (const route of available) {
        const option = document.createElement("option");
        option.value = route.ID;
        option.textContent = `${providerLabel(route.ProviderPluginID)} · ${route.DisplayName} (${route.AccessMode})`;
        els.routePicker.append(option);
    }
    els.routePicker.disabled = available.length === 0;
    els.addRoute.disabled = available.length === 0;
}
function limitLabel(costModel) {
    if (costModel.includes("license"))
        return "license quota";
    if (costModel.includes("free"))
        return "free quota";
    if (costModel.includes("local"))
        return "local compute";
    return "tracked quota";
}
function licenseLabel(route) {
    if (route.RequiresLicense)
        return `license: required · ${route.AccessMode}`;
    if (route.AccessMode.includes("free"))
        return `license: free tier · ${route.AccessMode}`;
    if (route.AccessMode === "local")
        return "license: local runtime";
    return `license: not required · ${route.AccessMode}`;
}
function accountLabel(profile) {
    if (!profile)
        return "account: not connected";
    const suffix = profile.IsDefault ? " · default" : "";
    return `account: ${profile.DisplayName}${suffix}`;
}
function renderRoutes(items) {
    els.routes.replaceChildren();
    const list = document.createElement("div");
    list.className = "registry-list";
    for (const item of items) {
        const row = document.createElement("div");
        row.className = "registry-item";
        const auth = item.RequiresAPIKey ? "API key" : item.RequiresLicense ? "License" : "No license";
        row.innerHTML = `<strong>${escapeHTML(providerLabel(item.ProviderPluginID))}</strong><div class="meta">${escapeHTML(item.DisplayName)} · ${escapeHTML(item.Transport)} · ${escapeHTML(auth)}</div><span class="pill">${escapeHTML(item.AccessMode)}</span>`;
        list.append(row);
    }
    els.routes.append(list);
}
function formatDate(value) {
    if (!value)
        return "";
    return new Intl.DateTimeFormat(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
    }).format(new Date(value));
}
function escapeHTML(value) {
    return String(value)
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;");
}
els.newSession.addEventListener("click", createSession);
els.navToggle.addEventListener("click", toggleNavigation);
els.projectToggle.addEventListener("click", toggleProjectChats);
els.projectMenuButton.addEventListener("click", toggleProjectMenu);
els.createProject.addEventListener("click", createProject);
els.renameProject.addEventListener("click", renameProject);
els.openSearch.addEventListener("click", openSearchModal);
els.searchBackdrop.addEventListener("click", closeSearchModal);
els.sessionSearch.addEventListener("input", searchSessions);
els.sessionSearch.addEventListener("keydown", (event) => {
    if (event.key === "Escape")
        closeSearchModal();
});
els.activeRoute.addEventListener("click", toggleProjectRoutes);
els.usageToggle.addEventListener("click", toggleAIUsage);
els.reasoningToggle.addEventListener("click", toggleReasoning);
els.addRoute.addEventListener("click", addProjectRoute);
els.composer.addEventListener("submit", sendMessage);
els.input.addEventListener("keydown", handleComposerKeydown);
els.modelPicker.addEventListener("click", toggleModelMenu);
els.modelEffort.addEventListener("click", toggleThinkingEffort);
els.modelSearch.addEventListener("input", () => renderModelMenu(modelOptions()));
els.workspaceTabs.addEventListener("click", (event) => {
    const button = event.target.closest("[data-workspace-tab]");
    if (!button)
        return;
    switchWorkspaceTab(button.dataset.workspaceTab);
});
els.newTerminalTab.addEventListener("click", newTerminalTab);
els.terminalCwd.addEventListener("change", () => {
    activeTerminalTab().workingDir = els.terminalCwd.value.trim();
});
els.terminalHistory.addEventListener("change", () => {
    if (els.terminalHistory.value)
        els.terminalCommand.value = els.terminalHistory.value;
});
els.terminalStop.addEventListener("click", stopTerminalCommand);
els.terminalForm.addEventListener("submit", queueTerminalCommand);
els.terminalRun.addEventListener("click", runTerminalCommand);
els.terminalCancel.addEventListener("click", cancelTerminalCommand);
els.newFileTab.addEventListener("click", newFileTab);
els.fileOpenForm.addEventListener("submit", openFileInActiveTab);
els.fileRecent.addEventListener("change", () => {
    if (els.fileRecent.value)
        openFilePath(els.fileRecent.value);
});
els.fileReload.addEventListener("click", reloadActiveFile);
switchWorkspaceTab(state.activeWorkspaceTab);
renderTerminalPanel();
renderFilePanel();
loadState().catch((error) => {
    els.messages.textContent = error.message;
});
