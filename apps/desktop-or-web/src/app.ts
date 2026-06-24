if (new URLSearchParams(window.location.search).get("desktop") === "1" || (window as any).ergoLoom) {
  document.documentElement.classList.add("desktop-shell");
}

function installDesktopWindowChrome() {
  const bridge = (window as any).ergoLoom;
  const hitbox = document.querySelector("#window-titlebar-hitbox");
  if (!bridge?.toggleMaximize || !hitbox) return;
  hitbox.addEventListener("dblclick", (event) => {
    event.preventDefault();
    bridge.toggleMaximize();
  });
}

const state = {
  sessions: [],
  projects: [],
  routes: [],
  project: null,
  selectedProjectId: savedProjectID(),
  selectedProviderIds: [],
  moderatorPreference: null,
  projectRoutes: [],
  profiles: [],
  models: [],
  usage: [],
  tools: [],
  authStatuses: [],
  diagnostics: null,
  providerChats: [],
  providerRefreshing: false,
  lastProviderRefreshAt: 0,
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
  chatQueue: [],
  queueDragId: "",
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

const warmBuildPrompts = [
  "오늘은 작은 맥락 하나가 큰 실마리가 될지도 몰라요.",
  "흩어진 생각을 천천히 엮어볼까요?",
  "가벼운 질문 하나로 오늘의 작업을 열어봐요.",
  "지금 손에 잡히는 것부터 함께 풀어봐요.",
  "복잡한 일도 한 줄의 대화에서 시작돼요.",
  "오늘의 작업 실을 어디서부터 잡아볼까요?",
  "조용히 쌓아둔 맥락을 다음 행동으로 이어봐요.",
  "조금 막힌 곳이 있다면 거기서부터 시작해도 좋아요.",
  "작은 수정 하나가 좋은 흐름을 만들 수 있어요.",
  "오늘의 프로젝트에 필요한 첫 매듭을 지어볼까요?",
  "아직 흐릿한 아이디어도 괜찮아요. 같이 선명하게 해봐요.",
  "생각의 가지를 나누고 다시 엮을 준비가 됐어요.",
  "가장 귀찮은 부분부터 덜어내볼까요?",
  "오늘은 어떤 대화를 작업으로 바꿔볼까요?",
  "작은 단서라도 충분해요. 거기서부터 이어갈게요.",
  "지금 필요한 맥락을 한곳에 모아봐요.",
  "작업의 첫 문장을 편하게 던져주세요.",
  "잠깐 멈춘 흐름을 다시 이어볼까요?",
  "오늘의 실마리는 이미 가까이에 있을지도 몰라요.",
  "작은 질문으로 큰 구조를 열어봐요.",
  "어떤 가지를 뻗고, 어떤 가지를 합칠까요?",
  "잘 정리되지 않은 생각도 좋은 출발점이에요.",
  "오늘의 작업을 덜 외롭게 만들어볼게요.",
  "가장 먼저 확인하고 싶은 걸 말해주세요.",
  "프로젝트의 결을 살피며 천천히 시작해봐요.",
  "지금 떠오른 걸 그대로 적어도 괜찮아요.",
  "좋은 답은 종종 작은 맥락 정리에서 나와요.",
  "오늘은 어떤 흐름을 저장하고 싶나요?",
  "대화의 실을 하나씩 고르게 펴볼까요?",
  "작은 자동화 하나로 하루가 가벼워질 수 있어요.",
  "막연한 계획을 실행 가능한 순서로 바꿔봐요.",
  "오늘의 작업 나무에 새 가지를 내볼까요?",
  "바로 만들지 않아도 좋아요. 먼저 살펴볼 수 있어요.",
  "이 프로젝트가 지금 어디에 서 있는지 같이 볼까요?",
  "복잡한 선택지를 차분히 비교해봐요.",
  "지금 가장 중요한 한 가지를 찾아볼까요?",
  "작업의 온도를 낮추고, 흐름을 정리해봐요.",
  "오늘의 질문을 내일의 도구로 엮어봐요.",
  "조금 느려도 단단하게 가면 돼요.",
  "흐트러진 세션들을 한 장의 지도로 만들어봐요.",
  "지금 가진 재료로 충분히 시작할 수 있어요.",
  "작업을 나누고, 필요한 곳에서 다시 합쳐봐요.",
  "오늘은 어떤 맥락을 잃어버리지 않게 할까요?",
  "작은 확인부터 시작하면 길이 보일 거예요.",
  "생각이 많을수록 첫 줄은 짧아도 좋아요.",
  "필요한 도구와 모델을 골라 작업을 열어봐요.",
  "오늘의 대화를 작업의 기억으로 남겨봐요.",
  "브랜치처럼 갈라진 생각을 함께 정리해요.",
  "다시 돌아올 수 있는 맥락을 만들어둘게요.",
  "가볍게 던진 요청도 충분히 좋은 시작이에요.",
  "지금 프로젝트에 필요한 다음 움직임은 뭘까요?",
  "오늘은 덜 헤매고 더 잘 이어가봐요.",
  "작업의 실밥을 하나씩 다듬어볼까요?",
  "세션과 지식을 한곳에서 차분히 엮어봐요.",
  "무엇을 만들지 아직 몰라도, 함께 찾을 수 있어요.",
  "작은 불편을 좋은 기능으로 바꿔봐요.",
  "현재 맥락을 기준으로 다음 행동을 골라봐요.",
  "오늘의 작업을 조금 더 부드럽게 시작해요.",
  "아이디어가 가지를 치면, 필요한 만큼 잡아둘게요.",
  "지금 필요한 답보다 필요한 흐름을 먼저 볼 수도 있어요.",
  "작업이 흩어지기 전에 여기서 붙잡아봐요.",
  "오늘은 어떤 반복을 줄여볼까요?",
  "문제의 모서리를 천천히 만져봐요.",
  "대화 하나가 좋은 작업 단위가 될 수 있어요.",
  "필요한 모델들을 고르고 같은 맥락에서 시작해봐요.",
  "오늘의 빌드는 조용한 정리에서 출발해도 좋아요.",
  "어떤 파일, 어떤 지침, 어떤 모델로 시작할까요?",
  "작업의 뿌리를 확인하고 가지를 뻗어봐요.",
  "한 번에 완성하지 않아도 괜찮아요. 이어가면 돼요.",
  "좋은 컨텍스트는 좋은 답을 부릅니다.",
  "작업의 다음 매듭을 여기서 묶어봐요.",
  "지금 생각나는 이름 없는 일을 맡겨주세요.",
  "작고 정확한 진전 하나를 만들어봐요.",
  "프로젝트의 오늘 기분을 살펴볼까요?",
  "어제의 맥락과 오늘의 선택을 이어봐요.",
  "작업의 길을 잃지 않도록 표시를 남겨둘게요.",
  "지금은 초안이어도 괜찮아요. 함께 다듬으면 돼요.",
  "좋은 질문은 이미 반쯤 만들어진 도구예요.",
  "오늘의 흐름을 가볍게 열어봅시다.",
  "무거운 작업도 작은 조각으로 나눠볼게요.",
  "당장 필요한 것과 나중에 필요한 것을 구분해봐요.",
  "한 프로젝트 안의 여러 AI를 한 자리에서 조율해봐요.",
  "오늘은 어떤 세션을 합치고, 어떤 세션을 나눌까요?",
  "작업의 중심을 다시 잡아볼까요?",
  "좋은 시작은 대개 부담 없는 한 문장이에요.",
  "지금 여기 있는 맥락만으로도 충분히 시작할 수 있어요.",
  "오늘의 문제를 내일의 지식으로 남겨봐요.",
  "작업의 가지가 많아질수록 뿌리를 함께 챙길게요.",
  "먼저 살펴보고, 그다음 움직여도 늦지 않아요.",
  "오늘의 작은 선택이 프로젝트의 결을 만듭니다.",
  "말로 풀어도 좋고, 파일로 시작해도 좋아요.",
  "여러 모델의 시선을 한 맥락에 모아봐요.",
  "작업의 실을 끊지 않고 이어갈 준비가 됐어요.",
  "오늘은 어디부터 가볍게 풀어볼까요?",
  "느슨한 아이디어를 단단한 다음 행동으로 바꿔봐요.",
  "프로젝트의 기억을 잃지 않게 함께 엮어둘게요.",
  "작업을 시작하기 좋은 조용한 순간이에요.",
  "지금 필요한 만큼만 복잡해져도 괜찮아요.",
  "오늘의 대화를 오래 쓸 수 있는 맥락으로 만들어봐요.",
  "작은 불확실성을 하나씩 걷어내볼까요?",
  "좋아요. 오늘도 천천히, 하지만 분명하게 시작해봐요.",
];

function q(selector): any {
  const element = document.querySelector(selector);
  if (!element) {
    throw new Error(`Missing element: ${selector}`);
  }
  return element;
}

const els = {
  shell: q("#app-shell"),
  windowTitlebarHitbox: q("#window-titlebar-hitbox"),
  sessions: q("#session-list"),
  messages: q("#messages"),
  chatQueueRail: q("#chat-queue-rail"),
  chatQueueList: q("#chat-queue-list"),
  chatQueueCount: q("#chat-queue-count"),
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
  appModal: q("#app-modal"),
  appModalForm: q("#app-modal-form"),
  appModalBackdrop: q("#app-modal-backdrop"),
  appModalTitle: q("#app-modal-title"),
  appModalMessage: q("#app-modal-message"),
  appModalInput: q("#app-modal-input"),
  appModalClose: q("#app-modal-close"),
  appModalCancel: q("#app-modal-cancel"),
  appModalConfirm: q("#app-modal-confirm"),
  projectToggle: q("#project-toggle"),
  projectMenuButton: q("#project-menu-button"),
  projectMenu: q("#project-menu"),
  renameProject: q("#rename-project"),
  deleteProject: q("#delete-project"),
  aiUsagePanel: q("#ai-usage-panel"),
  usageToggle: q("#usage-toggle"),
  providerRefresh: q("#provider-refresh"),
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
  runtimeDiagnostics: q("#runtime-diagnostics"),
  providerThreadList: q("#provider-thread-list"),
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
  const projectQuery = state.selectedProjectId ? `?projectId=${encodeURIComponent(state.selectedProjectId)}` : "";
  const data = await request(`/api/state${projectQuery}`);
  state.sessions = data.sessions || [];
  state.projects = data.projects || [];
  state.routes = data.routes || [];
  state.project = data.project || null;
  state.moderatorPreference = data.moderatorPreference || null;
  state.projectRoutes = data.projectRoutes || [];
  state.profiles = data.profiles || [];
  state.models = data.models || [];
  state.usage = data.usage || [];
  state.tools = data.tools || [];
  state.authStatuses = data.auth || [];
  state.diagnostics = data.diagnostics || null;
  state.lastProviderRefreshAt = Date.now();
  state.selectedProjectId = state.project?.ID || "";
  saveProjectID(state.selectedProjectId);
  if (state.selectedSessionId && !state.sessions.some((session) => session.ID === state.selectedSessionId)) {
    state.selectedSessionId = null;
  }
  normalizeSelectedProviders();
  renderProjectName();
  renderProjects();
  renderSessions();
  renderProjectRoutes();
  renderRoutePicker();
  renderModelPicker();
  renderNavUsage();
  renderProviderRefreshState();
  renderWorkspaceActivity();
  renderAuthStatuses();
  renderRuntimeDiagnostics();
  renderProviderThreads();
  renderToolRegistrySummary();
  renderTerminalPanel();
  renderFilePanel();
  renderRegistry(els.providers, groupedProviderRegistry(data.providers || []));
  renderRegistry(els.agents, data.agents || []);
  renderRoutes(state.routes);
  renderUsage(state.usage);

  if (!state.selectedSessionId && state.sessions.length > 0) {
    await selectSession(state.sessions[0].ID);
  } else if (!state.selectedSessionId) {
    renderEmptyChat();
  }
}

async function refreshProviderStatus(options: any = {}) {
  if (state.providerRefreshing) return;
  state.providerRefreshing = true;
  renderProviderRefreshState();
  try {
    await loadState();
    if (!options.silent) {
      appendWorkspaceEvent("result", { toolName: "provider.refresh", text: "Provider status refreshed" });
    }
  } catch (error) {
    if (!options.silent) {
      appendWorkspaceEvent("error", { toolName: "provider.refresh", text: error.message || String(error) });
    }
  } finally {
    state.providerRefreshing = false;
    renderProviderRefreshState();
  }
}

async function refreshProviderStatusIfStale() {
  if (Date.now() - state.lastProviderRefreshAt < 30000) return;
  await refreshProviderStatus({ silent: true });
}

function renderProviderRefreshState() {
  els.providerRefresh.classList.toggle("refreshing", state.providerRefreshing);
  els.providerRefresh.disabled = state.providerRefreshing;
}

function savedProjectID() {
  return window.localStorage.getItem("ergo-loom:selected-project") || "";
}

function saveProjectID(projectID) {
  if (!projectID) return;
  window.localStorage.setItem("ergo-loom:selected-project", projectID);
}

async function selectProject(projectID) {
  if (!projectID || projectID === state.project?.ID) return;
  state.selectedProjectId = projectID;
  state.selectedSessionId = null;
  state.showAllSessions = false;
  saveProjectID(projectID);
  await loadState();
}

function renderProjectName() {
  if (!els.projectName) return;
  els.projectName.textContent = state.project?.DisplayName || "Default Project";
}

async function addProjectRoute() {
  if (!state.project || !els.routePicker.value) return;
  await addRouteToCurrentProject(els.routePicker.value);
}

async function addRouteToCurrentProject(routeID) {
  if (!state.project || !routeID) return;
  await request(`/api/projects/${encodeURIComponent(state.project.ID)}/routes`, {
    method: "POST",
    body: JSON.stringify({ routeId: routeID }),
  });
  await loadState();
}

async function addProviderRoute(providerID) {
  const existing = state.projectRoutes.find((item) => item.Enabled && item.Route.ProviderPluginID === providerID);
  if (existing) return;
  const route = state.routes
    .filter((item) => item.ProviderPluginID === providerID)
    .sort((a, b) => routeStatusRank(a) - routeStatusRank(b))[0];
  if (!route) return;
  await addRouteToCurrentProject(route.ID);
}

async function addModelProvider(providerID) {
  await addProviderRoute(providerID);
  const groupID = providerGroupID(providerID);
  if (state.selectedSessionId) {
    const next = [...new Set([...state.selectedProviderIds, groupID])];
    await request(`/api/sessions/${encodeURIComponent(state.selectedSessionId)}/providers`, {
      method: "PATCH",
      body: JSON.stringify({ providerGroupIds: next }),
    });
    state.selectedProviderIds = next;
    saveSelectedProviderIDs(next);
    await loadState();
    await selectSession(state.selectedSessionId, { resetActivity: false });
    return;
  }
  const next = [...new Set([...state.selectedProviderIds, groupID])];
  state.selectedProviderIds = next;
  saveSelectedProviderIDs(next);
  renderModelPicker();
  renderEmptyChat();
}

function routeStatusRank(route) {
  if (route.Status === "available") return 0;
  if (route.SupportsHandoff) return 1;
  return 2;
}

async function createProject() {
  const displayName = String(await appPrompt({
    title: "Create project",
    placeholder: "Project name",
    confirmLabel: "Create",
  }) || "").trim();
  if (!displayName) return;
  const data = await request("/api/projects", {
    method: "POST",
    body: JSON.stringify({ displayName }),
  });
  await selectProject(data.project.ID);
}

async function renameProject() {
  if (!state.project) return;
  const displayName = String(await appPrompt({
    title: "Rename project",
    value: state.project.DisplayName,
    placeholder: "Project name",
    confirmLabel: "Rename",
  }) || "").trim();
  if (!displayName || displayName === state.project.DisplayName) return;
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

async function deleteCurrentProject() {
  if (!state.project || state.project.IsDefault) {
    await appAlert({
      title: "Default project",
      message: "Default Project는 삭제할 수 없습니다.",
    });
    els.projectMenu.hidden = true;
    return;
  }
  const confirmed = await appConfirm({
    title: "Delete project",
    message: `"${state.project.DisplayName}" 프로젝트와 하위 채팅을 삭제합니다. 이 작업은 되돌릴 수 없습니다.`,
    confirmLabel: "Delete",
    cancelLabel: "Cancel",
    danger: true,
  });
  if (!confirmed) return;
  await request(`/api/projects/${encodeURIComponent(state.project.ID)}`, {
    method: "DELETE",
  });
  state.selectedProjectId = "";
  state.selectedSessionId = null;
  saveProjectID("");
  els.projectMenu.hidden = true;
  await loadState();
}

async function removeProjectRoute(routeId) {
  if (!state.project) return;
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

function appPrompt({ title, message = "", value = "", placeholder = "", confirmLabel = "OK", cancelLabel = "Cancel" }) {
  return new Promise((resolve) => {
    els.appModalTitle.textContent = title;
    els.appModalMessage.textContent = message;
    els.appModalMessage.hidden = !message;
    els.appModalInput.hidden = false;
    els.appModalInput.value = value;
    els.appModalInput.placeholder = placeholder;
    els.appModalConfirm.textContent = confirmLabel;
    els.appModalCancel.textContent = cancelLabel;
    els.appModal.hidden = false;

    const cleanup = (result) => {
      els.appModal.hidden = true;
      els.appModalForm.removeEventListener("submit", onSubmit);
      els.appModalCancel.removeEventListener("click", onCancel);
      els.appModalClose.removeEventListener("click", onCancel);
      els.appModalBackdrop.removeEventListener("click", onCancel);
      document.removeEventListener("keydown", onKeyDown);
      resolve(result);
    };
    const onSubmit = (event) => {
      event.preventDefault();
      cleanup(els.appModalInput.value);
    };
    const onCancel = () => cleanup(null);
    const onKeyDown = (event) => {
      if (event.key === "Escape") cleanup(null);
    };

    els.appModalForm.addEventListener("submit", onSubmit);
    els.appModalCancel.addEventListener("click", onCancel);
    els.appModalClose.addEventListener("click", onCancel);
    els.appModalBackdrop.addEventListener("click", onCancel);
    document.addEventListener("keydown", onKeyDown);
    requestAnimationFrame(() => els.appModalInput.focus());
  });
}

function appAlert({ title = "Ergo Loom", message = "", confirmLabel = "OK" }) {
  return new Promise((resolve) => {
    els.appModalTitle.textContent = title;
    els.appModalMessage.textContent = message;
    els.appModalMessage.hidden = !message;
    els.appModalInput.hidden = true;
    els.appModalConfirm.textContent = confirmLabel;
    els.appModalCancel.textContent = "Cancel";
    els.appModalCancel.hidden = true;
    els.appModal.hidden = false;

    const cleanup = () => {
      els.appModal.hidden = true;
      els.appModalCancel.hidden = false;
      els.appModalForm.removeEventListener("submit", onSubmit);
      els.appModalClose.removeEventListener("click", onSubmit);
      els.appModalBackdrop.removeEventListener("click", onSubmit);
      document.removeEventListener("keydown", onKeyDown);
      resolve(true);
    };
    const onSubmit = (event) => {
      event?.preventDefault?.();
      cleanup();
    };
    const onKeyDown = (event) => {
      if (event.key === "Escape" || event.key === "Enter") cleanup();
    };

    els.appModalForm.addEventListener("submit", onSubmit);
    els.appModalClose.addEventListener("click", onSubmit);
    els.appModalBackdrop.addEventListener("click", onSubmit);
    document.addEventListener("keydown", onKeyDown);
    requestAnimationFrame(() => els.appModalConfirm.focus());
  });
}

function appConfirm({ title = "Ergo Loom", message = "", confirmLabel = "OK", cancelLabel = "Cancel", danger = false }) {
  return new Promise((resolve) => {
    els.appModalTitle.textContent = title;
    els.appModalMessage.textContent = message;
    els.appModalMessage.hidden = !message;
    els.appModalInput.hidden = true;
    els.appModalConfirm.textContent = confirmLabel;
    els.appModalCancel.textContent = cancelLabel;
    els.appModalCancel.hidden = false;
    els.appModalConfirm.classList.toggle("danger", danger);
    els.appModal.hidden = false;

    const cleanup = (result) => {
      els.appModal.hidden = true;
      els.appModalConfirm.classList.remove("danger");
      els.appModalForm.removeEventListener("submit", onSubmit);
      els.appModalCancel.removeEventListener("click", onCancel);
      els.appModalClose.removeEventListener("click", onCancel);
      els.appModalBackdrop.removeEventListener("click", onCancel);
      document.removeEventListener("keydown", onKeyDown);
      resolve(result);
    };
    const onSubmit = (event) => {
      event?.preventDefault?.();
      cleanup(true);
    };
    const onCancel = () => cleanup(false);
    const onKeyDown = (event) => {
      if (event.key === "Escape") cleanup(false);
      if (event.key === "Enter") cleanup(true);
    };

    els.appModalForm.addEventListener("submit", onSubmit);
    els.appModalCancel.addEventListener("click", onCancel);
    els.appModalClose.addEventListener("click", onCancel);
    els.appModalBackdrop.addEventListener("click", onCancel);
    document.addEventListener("keydown", onKeyDown);
    requestAnimationFrame(() => els.appModalConfirm.focus());
  });
}

function appChoice({ title = "Ergo Loom", message = "", primaryLabel = "OK", secondaryLabel = "Cancel", primaryValue = "primary", secondaryValue = "secondary", dangerSecondary = false }) {
  return new Promise((resolve) => {
    els.appModalTitle.textContent = title;
    els.appModalMessage.textContent = message;
    els.appModalMessage.hidden = !message;
    els.appModalInput.hidden = true;
    els.appModalConfirm.textContent = primaryLabel;
    els.appModalCancel.textContent = secondaryLabel;
    els.appModalCancel.hidden = false;
    els.appModalCancel.classList.toggle("danger", dangerSecondary);
    els.appModal.hidden = false;

    const cleanup = (result) => {
      els.appModal.hidden = true;
      els.appModalCancel.classList.remove("danger");
      els.appModalForm.removeEventListener("submit", onPrimary);
      els.appModalCancel.removeEventListener("click", onSecondary);
      els.appModalClose.removeEventListener("click", onClose);
      els.appModalBackdrop.removeEventListener("click", onClose);
      document.removeEventListener("keydown", onKeyDown);
      resolve(result);
    };
    const onPrimary = (event) => {
      event?.preventDefault?.();
      cleanup(primaryValue);
    };
    const onSecondary = () => cleanup(secondaryValue);
    const onClose = () => cleanup(null);
    const onKeyDown = (event) => {
      if (event.key === "Escape") cleanup(null);
      if (event.key === "Enter") cleanup(primaryValue);
    };

    els.appModalForm.addEventListener("submit", onPrimary);
    els.appModalCancel.addEventListener("click", onSecondary);
    els.appModalClose.addEventListener("click", onClose);
    els.appModalBackdrop.addEventListener("click", onClose);
    document.addEventListener("keydown", onKeyDown);
    requestAnimationFrame(() => els.appModalConfirm.focus());
  });
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
  els.projectMenu.hidden = true;
  normalizeSelectedProviders();
  const data = await request("/api/sessions", {
    method: "POST",
    body: JSON.stringify({
      title: "New chat",
      projectId: state.project?.ID || state.selectedProjectId || "default",
      providerGroupIds: state.selectedProviderIds,
    }),
  });
  state.selectedSessionId = data.session.ID;
  await loadState();
  await selectSession(data.session.ID);
  els.input.focus();
}

async function selectSession(sessionId, options: { resetActivity?: boolean } = {}) {
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
  state.providerChats = data.providerChats || [];
  state.selectedProviderIds = Array.isArray(data.providerGroupIds) ? data.providerGroupIds : [];
  saveSelectedProviderIDs(state.selectedProviderIds);
  // Sync queue from server: replace items for this session with server items
  const otherSessionItems = state.chatQueue.filter((item) => item.sessionId !== sessionId);
  const serverQueueItems = (data.queueItems || []).map((item) => ({
    id: item.ID,
    sessionId,
    content: item.Content,
    mode: item.Mode,
    routeId: item.RouteID,
    modelId: item.ModelID,
    thinkingEffort: item.ThinkingEffort,
    modelLabel: modelLabelForIds(item.ModelID, item.RouteID),
    createdAt: new Date(item.CreatedAt),
  }));
  state.chatQueue = [...otherSessionItems, ...serverQueueItems];
  renderContextMeter();
  renderProviderThreads();
  renderModelPicker();
  renderMessages(data.messages || [], data.events || []);
  renderChatQueue();
  closeSearchModal();
}

async function sendMessage(event) {
  event.preventDefault();
  const content = els.input.value.trim();
  if (!content) return;
  await refreshProviderStatusIfStale();
  if (!selectedModelOption()) {
    await appAlert({
      title: "No model selected",
      message: "Select an available model for this project first.",
    });
    return;
  }
  if (!state.selectedSessionId) {
    await createSession();
  }

  els.input.value = "";
  if (isChatRunning()) {
    await enqueueChat(content);
    return;
  }
  await runChatInput(content);
}

async function runChatInput(content, options: any = {}) {
  setComposerBusy(true);
  state.runningSessionId = state.selectedSessionId;
  renderSessions();
  try {
    await streamMessage(state.selectedSessionId, content, options.selection);
    await loadState();
    await selectSession(state.selectedSessionId, { resetActivity: false });
  } catch (error) {
    appendActivityEvent("error", { text: error.message || String(error), toolName: "chat" });
  } finally {
    state.runningSessionId = "";
    renderSessions();
    setComposerBusy(false);
    drainChatQueue();
  }
}

async function submitChatText(content) {
  const text = String(content || "").trim();
  if (!text) return;
  await refreshProviderStatusIfStale();
  if (!state.selectedSessionId) {
    await createSession();
  }
  if (isChatRunning()) {
    await enqueueChat(text);
    return;
  }
  await runChatInput(text);
}

function setComposerBusy(busy) {
  if (!busy && state.runningSessionId) {
    state.runningSessionId = "";
    renderSessions();
  }
  els.input.disabled = false;
  els.sendButton.disabled = !selectedModelOption();
}

function renderChatQueue() {
  const items = state.chatQueue.filter((item) => item.sessionId === state.selectedSessionId);
  els.chatQueueRail.hidden = items.length === 0;
  els.chatQueueCount.textContent = String(items.length);
  els.chatQueueList.replaceChildren();
  for (const item of items) {
    const row = document.createElement("article");
    row.className = `chat-queue-item mode-${item.mode}`;
    row.draggable = true;
    row.dataset.queueId = item.id;
    row.innerHTML = `
      <div class="chat-queue-grip" aria-hidden="true">⋮⋮</div>
      <div class="chat-queue-body">
        <div class="chat-queue-text">${escapeHTML(item.content)}</div>
        <div class="chat-queue-meta">${escapeHTML(queueModeLabel(item.mode))} · ${escapeHTML(item.modelLabel)}</div>
        <div class="chat-queue-actions">
          <button type="button" data-queue-action="steer" title="현재 답변 완료 전에 이어지는 입력으로 밀어넣기">${item.mode === "steer" ? "Steering" : "Steer"}</button>
          <button type="button" data-queue-action="parallel" title="다른 모델이 같은 컨텍스트로 미리 준비하는 후보로 표시">${item.mode === "parallel" ? "Parallel" : "Parallel"}</button>
          <button type="button" data-queue-action="remove">Remove</button>
        </div>
      </div>
    `;
    row.addEventListener("dragstart", () => {
      state.queueDragId = item.id;
      row.classList.add("dragging");
    });
    row.addEventListener("dragend", () => {
      state.queueDragId = "";
      row.classList.remove("dragging");
    });
    row.addEventListener("dragover", (event) => {
      event.preventDefault();
      reorderQueuedChat(state.queueDragId, item.id);
    });
    row.querySelectorAll("[data-queue-action]").forEach((button) => {
      const actionButton = button as HTMLButtonElement;
      actionButton.addEventListener("click", () => updateQueuedChat(item.id, actionButton.dataset.queueAction));
    });
    els.chatQueueList.append(row);
  }
}

async function updateQueuedChat(id, action) {
  const sessionId = state.selectedSessionId;
  if (action === "remove") {
    state.chatQueue = state.chatQueue.filter((item) => item.id !== id);
    renderChatQueue();
    await request(`/api/sessions/${encodeURIComponent(sessionId)}/queue`, {
      method: "PATCH",
      body: JSON.stringify({ itemId: id, status: "removed" }),
    }).catch(() => {});
  } else if (action === "steer" || action === "parallel") {
    state.chatQueue = state.chatQueue.map((item) => item.id === id ? { ...item, mode: action } : item);
    renderChatQueue();
    await request(`/api/sessions/${encodeURIComponent(sessionId)}/queue`, {
      method: "PATCH",
      body: JSON.stringify({ itemId: id, mode: action }),
    }).catch(() => {});
  }
}

function reorderQueuedChat(sourceId, targetId) {
  if (!sourceId || !targetId || sourceId === targetId) return;
  const sourceIndex = state.chatQueue.findIndex((item) => item.id === sourceId);
  const targetIndex = state.chatQueue.findIndex((item) => item.id === targetId);
  if (sourceIndex < 0 || targetIndex < 0) return;
  if (state.chatQueue[sourceIndex].sessionId !== state.selectedSessionId || state.chatQueue[targetIndex].sessionId !== state.selectedSessionId) return;
  const [source] = state.chatQueue.splice(sourceIndex, 1);
  state.chatQueue.splice(targetIndex, 0, source);
  renderChatQueue();
  const sessionItems = state.chatQueue.filter((item) => item.sessionId === state.selectedSessionId);
  request(`/api/sessions/${encodeURIComponent(state.selectedSessionId)}/queue`, {
    method: "PATCH",
    body: JSON.stringify({ itemIds: sessionItems.map((i) => i.id) }),
  }).catch(() => {});
}

function queueModeLabel(mode) {
  return mode === "parallel" ? "Parallel prep" : "Steering";
}

function isChatRunning() {
  return Boolean(state.runningSessionId && state.runningSessionId === state.selectedSessionId);
}

async function enqueueChat(content, mode = "steer") {
  const selected = selectedModelOption();
  const sessionId = state.selectedSessionId;
  const modelLabel = selected ? `${providerLabel(selected.model.ProviderPluginID)} · ${modelDisplayName(selected.model)}` : "No model";
  try {
    const data = await request(`/api/sessions/${encodeURIComponent(sessionId)}/queue`, {
      method: "POST",
      body: JSON.stringify({
        content,
        mode,
        routeId: selected?.routeId || "",
        modelId: selected?.model.ID || "",
        thinkingEffort: state.thinkingEffort,
      }),
    });
    const item = data.queueItem;
    state.chatQueue.push({
      id: item.ID,
      sessionId,
      content: item.Content,
      mode: item.Mode,
      routeId: item.RouteID,
      modelId: item.ModelID,
      thinkingEffort: item.ThinkingEffort,
      modelLabel,
      createdAt: new Date(item.CreatedAt),
    });
  } catch {
    // Fallback: add locally with temp id if API unavailable
    state.chatQueue.push({
      id: `queue-${Date.now()}-${Math.random().toString(16).slice(2)}`,
      sessionId,
      content,
      mode,
      routeId: selected?.routeId || "",
      modelId: selected?.model.ID || "",
      thinkingEffort: state.thinkingEffort,
      modelLabel,
      createdAt: new Date(),
    });
  }
  renderChatQueue();
}

async function drainChatQueue() {
  if (isChatRunning()) return;
  const nextIndex = state.chatQueue.findIndex((item) => item.sessionId === state.selectedSessionId);
  if (nextIndex < 0) {
    renderChatQueue();
    return;
  }
  const [next] = state.chatQueue.splice(nextIndex, 1);
  renderChatQueue();

  // Mark consumed on server (best effort)
  request(`/api/sessions/${encodeURIComponent(next.sessionId)}/queue`, {
    method: "PATCH",
    body: JSON.stringify({ itemId: next.id, status: "consumed" }),
  }).catch(() => {});

  if (next.mode === "parallel") {
    // Trigger parallel run in background and continue draining
    triggerParallelRun(next).catch(() => {});
    await drainChatQueue();
    return;
  }

  if (next.mode === "steer") {
    // Record steering event (best effort — run may have already finished)
    request(`/api/sessions/${encodeURIComponent(next.sessionId)}/steering`, {
      method: "POST",
      body: JSON.stringify({ content: next.content }),
    }).catch(() => {});
  }

  await runChatInput(next.content, {
    selection: {
      routeId: next.routeId,
      modelId: next.modelId,
      thinkingEffort: next.thinkingEffort,
    },
  });
}

async function triggerParallelRun(item) {
  const data = await request(`/api/sessions/${encodeURIComponent(item.sessionId)}/parallel`, {
    method: "POST",
    body: JSON.stringify({
      content: item.content,
      routeId: item.routeId,
      modelId: item.modelId,
      thinkingEffort: item.thinkingEffort,
    }),
  });
  appendActivityEvent("candidate", { candidateId: data.candidateId, text: "Parallel candidate running in background..." });
}

function queueTerminalCommand(event) {
  event.preventDefault();
  const command = els.terminalCommand.value.trim();
  if (!command) return;
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
  if (!command) return;
  const controller = new AbortController();
  terminalControllers.set(tab.id, controller);
  tab.running = true;
  if (!tab.history.includes(command)) tab.history.unshift(command);
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
  } catch (error) {
    runningLine.text = error.name === "AbortError" ? "cancelled by user" : error.message || String(error);
    runningLine.className = "error";
    appendWorkspaceEvent("error", { toolName: "zsh", command, text: error.message || String(error) });
    renderTerminalPanel();
  } finally {
    tab.running = false;
    terminalControllers.delete(tab.id);
    renderTerminalPanel();
    els.terminalRun.disabled = false;
  }
}

function stopTerminalCommand() {
  const tab = activeTerminalTab();
  const controller = terminalControllers.get(tab.id);
  if (controller) controller.abort();
}

async function streamMessage(sessionId, content, selectionOverride: any = {}) {
  const selected = selectedModelOption();
  const routeId = selectionOverride.routeId || selected?.routeId || "";
  const modelId = selectionOverride.modelId || selected?.model.ID || "";
  const thinkingEffort = selectionOverride.thinkingEffort || state.thinkingEffort;
  const response = await fetch(`/api/sessions/${encodeURIComponent(sessionId)}/messages/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      content,
      routeId,
      modelId,
      thinkingEffort,
    }),
  });
  if (!response.ok || !response.body) {
    throw new Error(`Request failed: ${response.status}`);
  }

  let assistantNode = null;
  let assistantContent = "";
  let lastStatus = "";
  let assistantActivity = null;
  resetReasoningStream();
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";
    for (const line of lines) {
      if (!line.trim()) continue;
      const event = JSON.parse(line);
      if (event.type === "message") {
        appendMessage(event.payload.Role, event.payload.Content);
      }
      if (event.type === "assistant_start") {
        assistantNode = appendMessage("assistant", "", { activityIndex: nextAssistantActivityIndex() });
        assistantActivity = ensureMessageActivity(assistantNode);
      }
      if (event.type === "assistant_delta" && assistantNode) {
        assistantContent += event.payload.text;
        renderMarkdown(assistantNode.querySelector(".content"), assistantContent.trimEnd());
        els.messages.scrollTop = els.messages.scrollHeight;
      }
      if (event.type === "assistant_status" && event.payload.text !== lastStatus) {
        lastStatus = event.payload.text;
        appendReasoning(event.payload.text);
        appendMessageActivity(assistantActivity, "status", event.payload);
      }
      if (event.type === "tool_start") {
        appendActivityEvent("tool", event.payload);
        appendMessageActivity(assistantActivity, "tool", event.payload);
      }
      if (event.type === "tool_result") {
        appendActivityEvent("result", event.payload);
        appendMessageActivity(assistantActivity, "result", event.payload);
      }
      if (event.type === "approval_request") {
        appendActivityEvent("approval", event.payload);
        appendMessageActivity(assistantActivity, "approval", event.payload);
        addToolApproval(event.payload);
      }
      if (event.type === "tool_error" || event.type === "turn_aborted") {
        if (event.payload.approvalId) {
          removeToolApproval(event.payload.approvalId);
        }
        appendActivityEvent("error", event.payload);
        appendMessageActivity(assistantActivity, "error", event.payload);
      }
      if (event.type === "error") {
        appendActivityEvent("error", { text: event.payload.message, toolName: "chat" });
        appendMessageActivity(assistantActivity, "error", { text: event.payload.message, toolName: "chat" });
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
      openSessionActions(session.ID);
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

async function openSessionActions(sessionID) {
  const session = state.sessions.find((item) => item.ID === sessionID);
  if (!session) return;
  const action = await appChoice({
    title: "Chat actions",
    message: session.Title,
    primaryLabel: "Rename",
    secondaryLabel: "Delete",
    primaryValue: "rename",
    secondaryValue: "delete",
    dangerSecondary: true,
  });
  if (action === "rename") {
    await renameSession(sessionID);
  }
  if (action === "delete") {
    await deleteSession(sessionID);
  }
}

async function renameSession(sessionID) {
  const session = state.sessions.find((item) => item.ID === sessionID);
  if (!session) return;
  const title = String(await appPrompt({
    title: "Rename chat",
    value: session.Title,
    placeholder: "Chat name",
    confirmLabel: "Rename",
  }) || "").trim();
  if (!title || title === session.Title) return;
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

async function deleteSession(sessionID) {
  const session = state.sessions.find((item) => item.ID === sessionID);
  if (!session) return;
  if (sessionID === state.runningSessionId) {
    await appAlert({
      title: "Chat is running",
      message: "실행 중인 채팅은 완료 후 삭제할 수 있습니다.",
    });
    return;
  }
  const confirmed = await appConfirm({
    title: "Delete chat",
    message: `"${session.Title}" 채팅을 삭제합니다. 이 작업은 되돌릴 수 없습니다.`,
    confirmLabel: "Delete",
    cancelLabel: "Cancel",
    danger: true,
  });
  if (!confirmed) return;
  await request(`/api/sessions/${encodeURIComponent(sessionID)}`, {
    method: "DELETE",
  });
  state.chatQueue = state.chatQueue.filter((item) => item.sessionId !== sessionID);
  if (state.selectedSessionId === sessionID) {
    state.selectedSessionId = null;
    resetReasoningStream();
    clearToolApprovals();
  }
  await loadState();
  if (!state.selectedSessionId && state.sessions.length > 0) {
    await selectSession(state.sessions[0].ID);
  } else if (state.sessions.length === 0) {
    renderEmptyChat();
    renderChatQueue();
  }
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
    row.addEventListener("click", () => selectProject(project.ID));
    els.projects.append(row);
  }
}

function searchSessions() {
  window.clearTimeout(state.searchTimer);
  state.searchTimer = window.setTimeout(async () => {
    const query = els.sessionSearch.value.trim();
    els.searchResults.replaceChildren();
    if (!query) return;
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
    } else {
      row.addEventListener("click", () => {
        closeSearchModal();
        selectProject(item.id);
      });
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

function renderMessages(messages, events = []) {
  els.messages.replaceChildren();
  if (messages.length === 0) {
    renderEmptyChat();
    return;
  }

  const eventsByActivity = messageEventsByActivity(events);
  let assistantIndex = 0;
  for (const message of messages) {
    if (message.Role === "assistant") assistantIndex += 1;
    const node = appendMessage(message.Role, message.Content, {
      activityIndex: message.Role === "assistant" ? assistantIndex : 0,
    });
    if (message.Role === "assistant") {
      restoreMessageActivity(node, assistantIndex, eventsByActivity.get(assistantIndex) || null);
    }
  }
  els.messages.scrollTop = els.messages.scrollHeight;
}

function appendMessage(roleName, text, options: any = {}) {
  const item = document.createElement("article");
  item.className = `message ${roleName}`;
  if (options.activityIndex) {
    item.dataset.activityIndex = String(options.activityIndex);
  }

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

function ensureMessageActivity(messageNode) {
  if (!messageNode) return null;
  let activity = messageNode.querySelector(".message-activity");
  if (!activity) {
    activity = document.createElement("div");
    activity.className = "message-activity";
    activity.dataset.activityIndex = messageNode.dataset.activityIndex || "";
    messageNode.append(activity);
  }
  return activity;
}

function appendMessageActivity(activity, kind, payload: any = {}, options: any = {}) {
  if (!activity) return;
  const row = kind === "status"
    ? messageActivityStatus(payload)
    : messageActivityDetails(kind, payload);
  activity.append(row);
  if (options.persistLocal === true) {
    persistMessageActivity(activity, kind, payload);
  }
  els.messages.scrollTop = els.messages.scrollHeight;
}

function restoreMessageActivity(messageNode, activityIndex, databaseEvents = null) {
  const events = databaseEvents || loadMessageActivities(state.selectedSessionId)
    .filter((item) => item.activityIndex === activityIndex);
  if (events.length === 0) return;
  const activity = ensureMessageActivity(messageNode);
  for (const event of events) {
    appendMessageActivity(activity, event.kind, event.payload, { persist: false });
  }
}

function messageEventsByActivity(events) {
  const map = new Map();
  for (const event of events || []) {
    const activityIndex = Number(event.activityIndex || event.ActivityIndex || 0);
    const kind = event.kind || event.Kind || "";
    if (!activityIndex || !kind) continue;
    const payload = parseMessageEventPayload(event.payloadJson || event.payloadJSON || event.PayloadJSON);
    if (!map.has(activityIndex)) map.set(activityIndex, []);
    map.get(activityIndex).push({ activityIndex, kind, payload });
  }
  return map;
}

function parseMessageEventPayload(value) {
  if (!value) return {};
  if (typeof value === "object") return value;
  try {
    return JSON.parse(value);
  } catch {
    return { text: String(value) };
  }
}

function persistMessageActivity(activity, kind, payload: any = {}) {
  const activityIndex = Number(activity.dataset.activityIndex || "0");
  if (!state.selectedSessionId || !activityIndex) return;
  const events = loadMessageActivities(state.selectedSessionId);
  events.push({
    activityIndex,
    kind,
    payload: compactActivityPayload(payload),
    createdAt: new Date().toISOString(),
  });
  saveMessageActivities(state.selectedSessionId, events.slice(-200));
}

function compactActivityPayload(payload: any = {}) {
  return {
    text: payload.text || "",
    toolName: payload.toolName || "",
    command: payload.command || "",
    status: payload.status || "",
    approvalId: payload.approvalId || "",
    sessionId: payload.sessionId || "",
    raw: payload.raw || null,
  };
}

function loadMessageActivities(sessionId) {
  if (!sessionId) return [];
  try {
    const raw = window.localStorage.getItem(messageActivityStorageKey(sessionId));
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function saveMessageActivities(sessionId, events) {
  if (!sessionId) return;
  window.localStorage.setItem(messageActivityStorageKey(sessionId), JSON.stringify(events));
}

function messageActivityStorageKey(sessionId) {
  return `ergo-loom:message-activity:${sessionId}`;
}

function nextAssistantActivityIndex() {
  return els.messages.querySelectorAll(".message.assistant").length + 1;
}

function messageActivityStatus(payload) {
  const row = document.createElement("div");
  row.className = "message-activity-status";
  row.textContent = payload.text || "Thinking...";
  return row;
}

function messageActivityDetails(kind, payload: any) {
  const details = document.createElement("details");
  details.className = `message-activity-details activity-${kind}`;
  details.open = kind === "approval" || kind === "error";

  const summary = document.createElement("summary");
  const icon = document.createElement("span");
  icon.className = "message-activity-icon";
  icon.textContent = activityIcon(kind);
  const label = document.createElement("span");
  label.className = "message-activity-label";
  label.textContent = messageActivityTitle(kind, payload);
  summary.append(icon, label);
  details.append(summary);

  const detailText = [payload.command, payload.text].filter(Boolean).join("\n").trim();
  if (detailText) {
    const detail = document.createElement("pre");
    detail.className = "message-activity-detail";
    detail.textContent = detailText;
    details.append(detail);
  }

  return details;
}

function messageActivityTitle(kind, payload: any) {
  const tool = payload.toolName || "tool";
  const command = payload.command ? ` ${payload.command}` : "";
  if (kind === "tool") return `실행 중인 ${tool}${command}`;
  if (kind === "result") return `완료한 ${tool}${command}`;
  if (kind === "approval") return `승인 대기 ${tool}${command}`;
  if (kind === "error") return `중단됨 ${tool}${command}`;
  return activityTitle(kind, payload);
}

function activityIcon(kind) {
  if (kind === "result") return "✓";
  if (kind === "approval") return "?";
  if (kind === "error") return "!";
  return ">";
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
    if (paragraph.length === 0) return;
    const p = document.createElement("p");
    appendInlineMarkdown(p, paragraph.join(" "));
    nodes.push(p);
    paragraph = [];
  };
  const flushList = () => {
    if (!list) return;
    nodes.push(list);
    list = null;
  };
  const flushCode = () => {
    if (!code) return;
    const pre = document.createElement("pre");
    const codeNode = document.createElement("code");
    if (codeLang) codeNode.dataset.lang = codeLang;
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
      } else {
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
      if (!list) list = document.createElement("ul");
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
    } else if (token.startsWith("`")) {
      const code = document.createElement("code");
      code.textContent = token.slice(1, -1);
      parent.append(code);
    } else {
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

function appendActivityEvent(kind, payload: any = {}) {
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
    } else {
      actions.innerHTML = `<span class="activity-status">Waiting for approval above composer</span>`;
    }
    item.append(actions);
  }

  els.reasoningStream.append(item);
  els.reasoningStream.scrollTop = els.reasoningStream.scrollHeight;
  els.reasoningCount.textContent = String(els.reasoningStream.children.length);
  return item;
}

function mirrorToolEventToTerminal(kind, payload: any = {}) {
  const command = payload.command || "";
  const text = payload.text || "";
  const toolName = String(payload.toolName || payload.toolId || "");
  const looksLikeCommand = command || /command|shell|zsh|exec/i.test(toolName);
  if (!looksLikeCommand) return;
  if (kind === "tool") {
    appendTerminalLine(command ? `$ ${command}` : `tool started · ${toolName}`, "muted");
    switchWorkspaceTab("terminals");
  } else if (kind === "result") {
    if (text) {
      for (const line of text.trimEnd().split("\n").slice(-80)) {
        appendTerminalLine(line);
      }
    }
  } else if (kind === "error") {
    appendTerminalLine(text || `tool interrupted · ${toolName}`, "error");
  } else if (kind === "approval") {
    switchWorkspaceTab("activity");
  }
}

function appendWorkspaceEvent(kind, payload: any = {}) {
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
  if (!state.authStatuses.length) return;
  for (const item of state.authStatuses) {
    const row = document.createElement("div");
    row.className = `auth-status-row ${authStatusClass(item)}`;
    row.innerHTML = `
      <div>
        <strong>${escapeHTML(item.label)}</strong>
        <span>${escapeHTML(item.accountLabel || item.detail || item.status || "")}</span>
      </div>
      <span>${escapeHTML(authStatusLabel(item))}</span>
    `;
    els.authStatusList.append(row);
  }
}

function authStatusClass(item) {
  if (item.connected) return "connected";
  if (item.status === "missing") return "missing";
  if (item.id === "claude") return "subscription-required";
  if (item.id === "github" || item.id === "copilot") return "auth-refresh";
  return "setup-required";
}

function authStatusLabel(item) {
  if (item.connected) return "Ready";
  if (item.id === "claude") return "Subscription setup";
  if (item.id === "github" || item.id === "copilot") return "Auth refresh";
  if (item.status === "missing") return "Missing";
  return "Needs setup";
}

function renderRuntimeDiagnostics() {
  els.runtimeDiagnostics.replaceChildren();
  const diagnostics = state.diagnostics;
  if (!diagnostics) return;

  const label = document.createElement("div");
  label.className = "workspace-panel-label";
  label.textContent = "Runtime Diagnostics";

  const list = document.createElement("div");
  list.className = "runtime-diagnostic-list";
  const summaryRows = [
    ["Mode", diagnostics.desktop ? "Desktop app" : "Browser dev"],
    ["Data", diagnostics.dataDir || "~/.ergo-loom"],
    ["Bridge", diagnostics.handoffBridge ? "handoff worker ready" : "not configured"],
  ];
  for (const [name, value] of summaryRows) {
    list.append(runtimeDiagnosticRow(name, value, value));
  }
  for (const item of diagnostics.executables || []) {
    list.append(runtimeDiagnosticRow(item.label, item.status === "ready" ? item.path : item.detail, item.path || item.detail, item.status));
  }
  if (diagnostics.path) {
    list.append(runtimeDiagnosticRow("PATH", diagnostics.path, diagnostics.path));
  }
  els.runtimeDiagnostics.append(label, list);
}

function runtimeDiagnosticRow(label, value, title = "", status = "") {
  const row = document.createElement("div");
  row.className = `runtime-diagnostic-row ${status || ""}`.trim();
  row.title = title || value || "";
  row.innerHTML = `
    <strong>${escapeHTML(label)}</strong>
    <span>${escapeHTML(value || "not available")}</span>
  `;
  return row;
}

function renderProviderThreads() {
  els.providerThreadList.replaceChildren();
  const label = document.createElement("div");
  label.className = "workspace-panel-label";
  label.textContent = "Provider Threads";
  const list = document.createElement("div");
  list.className = "provider-thread-items";
  if (!state.providerChats.length) {
    const empty = document.createElement("div");
    empty.className = "meta";
    empty.textContent = "No provider threads yet";
    list.append(empty);
  } else {
    for (const item of state.providerChats) {
      const row = document.createElement("div");
      row.className = "provider-thread-row";
      row.title = item.ExternalThreadID || "";
      row.innerHTML = `
        <strong>${escapeHTML(providerLabel(item.ProviderPluginID))}</strong>
        <span>${escapeHTML(providerThreadRouteLabel(item))}</span>
        <code>${escapeHTML(shortThreadID(item.ExternalThreadID))}</code>
      `;
      list.append(row);
    }
  }
  els.providerThreadList.append(label, list);
}

function providerThreadRouteLabel(item) {
  const route = state.routes.find((candidate) => candidate.ID === item.AccessRouteID);
  const model = state.models.find((candidate) => candidate.ID === item.ModelID);
  return [route?.DisplayName || item.AccessRouteID, model?.DisplayName || item.ModelID]
    .filter(Boolean)
    .join(" · ");
}

function shortThreadID(value) {
  const text = String(value || "");
  if (text.length <= 18) return text || "local";
  return `${text.slice(0, 8)}…${text.slice(-6)}`;
}

function renderToolRegistrySummary() {
  els.toolRegistryList.replaceChildren();
  if (!state.tools.length) return;
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
  if (state.pendingApprovals.length === 0) return;
  const label = document.createElement("div");
  label.className = "workspace-panel-label";
  label.textContent = "Pending approvals";
  els.workspaceApprovals.append(label);
  for (const approval of state.pendingApprovals) {
    els.workspaceApprovals.append(toolApprovalCard(approval));
  }
}

function addToolApproval(payload) {
  if (!payload.approvalId) return;
  removeToolApproval(payload.approvalId, false);
  state.pendingApprovals.push({
    ...payload,
    directInput: "",
    suggestions: approvalSuggestions(payload),
  });
  renderToolApprovals();
}

function removeToolApproval(approvalId, shouldRender = true) {
  const before = state.pendingApprovals.length;
  state.pendingApprovals = state.pendingApprovals.filter((item) => item.approvalId !== approvalId);
  if (shouldRender && state.pendingApprovals.length !== before) {
    renderToolApprovals();
  }
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
  const approveForSession = approvalLineButton(2, "승인", "다음부터 유사 명령어에 대해 허용", "accept_for_session", approval);
  const deny = approvalLineButton(3, "거절", "실행하지 않음", "decline", approval);

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
  const directButton = approvalActionButton("전송", approval, input);
  direct.append(input, directButton);

  const suggestions = document.createElement("div");
  suggestions.className = "tool-approval-suggestions";
  approval.suggestions.forEach((suggestion, index) => {
    suggestions.append(approvalSuggestionButton(index + 4, `제안${index + 1}`, suggestion, approval));
  });

  card.append(title, approve, approveForSession, deny, suggestions, direct);
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

function approvalSuggestionButton(index, label, value, payload) {
  const button = document.createElement("button");
  button.className = "tool-approval-line";
  button.type = "button";
  button.innerHTML = `
    <span class="tool-approval-marker">${index}</span>
    <strong>${escapeHTML(label)}:</strong>
    <span>${escapeHTML(value)}</span>
  `;
  button.addEventListener("click", () => sendApprovalAlternative(payload, value, button));
  return button;
}

function approvalActionButton(label, payload, input) {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  button.addEventListener("click", () => {
    payload.directInput = input.value;
    sendApprovalAlternative(payload, payload.directInput, button);
  });
  return button;
}

async function sendApprovalAlternative(payload, text, sourceButton) {
  const content = String(text || "").trim();
  if (!content) return;
  const resolved = await resolveApproval(payload, "decline", sourceButton, {
    workspaceKind: "approval",
    workspaceText: "Declined tool execution and sent alternative instruction",
  });
  if (!resolved) return;
  await submitChatText(content);
}

async function resolveApproval(payload, decision, sourceButton, options: any = {}) {
  if (!payload.approvalId) return;
  const card = sourceButton.closest(".tool-approval-card");
  for (const item of card.querySelectorAll("button, input")) item.disabled = true;
  try {
    await request(`/api/tool-approvals/${encodeURIComponent(payload.approvalId)}`, {
      method: "POST",
      body: JSON.stringify({
        decision,
        directInput: payload.directInput || "",
      }),
    });
    removeToolApproval(payload.approvalId, false);
    const approved = decision === "accept" || decision === "accept_for_session";
    appendWorkspaceEvent(options.workspaceKind || (approved ? "approval" : "error"), {
      ...payload,
      status: decision,
      text: options.workspaceText || approvalDecisionText(decision),
    });
    renderToolApprovals();
    return true;
  } catch (error) {
    if (isStaleApprovalError(error)) {
      removeToolApproval(payload.approvalId);
      appendWorkspaceEvent("error", {
        ...payload,
        status: "expired",
        text: "Approval request expired",
      });
      return true;
    }
    const status = document.createElement("span");
    status.className = "activity-status error";
    status.textContent = error.message || "Approval failed";
    card.append(status);
    for (const item of card.querySelectorAll("button, input")) item.disabled = false;
    return false;
  }
}

function approvalDecisionText(decision) {
  if (decision === "accept") return "Approved by user";
  if (decision === "accept_for_session") return "Approved similar commands for this session";
  return "Declined by user";
}

function isStaleApprovalError(error) {
  return String(error?.message || "").includes("approval request is no longer pending");
}

function approvalSuggestions(payload) {
  const raw = payload.raw || {};
  const fromPayload = Array.isArray(raw.suggestions) ? raw.suggestions.filter(Boolean) : [];
  if (fromPayload.length > 0) return fromPayload;
  return [
    "명령을 실행하지 않고 필요한 이유만 설명",
    "실행할 명령을 채팅에만 제안",
  ].filter(Boolean);
}

function activityTitle(kind, payload: any) {
  const tool = payload.toolName || "tool";
  if (kind === "error") return `Interrupted · ${tool}`;
  if (kind === "approval" && payload.status === "declined") return `Approval declined · ${tool}`;
  if (kind === "approval") return `Approval required · ${tool}${payload.sessionId ? ` · current chat` : ""}`;
  if (kind === "result") return `Tool result · ${tool}`;
  if (kind === "candidate") return `Parallel candidate · ${payload.candidateId || ""}`;
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
  state.providerChats = [];
  renderContextMeter();
  renderProviderThreads();
  els.messages.replaceChildren();
  renderChatQueue();
  const providerOptions = availableProviderOptions();
  const moderator = effectiveModerator(providerOptions);
  const home = document.createElement("section");
  home.className = "new-chat-home";
  home.innerHTML = `
    <div class="new-chat-main">
      <h2>${escapeHTML(randomWarmBuildPrompt())}</h2>
      <section class="new-chat-setup">
        <div class="new-chat-setup-head">
          <div>
            <strong>프로젝트</strong>
            <span>${escapeHTML(state.project?.DisplayName || "Default Project")}</span>
          </div>
          <span>${providerOptions.length} providers ready</span>
        </div>
        <div class="new-chat-setup-grid">
          <div class="new-chat-setup-block">
            <h3>프로젝트 경로</h3>
            <select class="project-context-select" aria-label="Project context">
              ${state.projects.map((project) => `
                <option value="${escapeHTML(project.ID)}" ${project.ID === state.project?.ID ? "selected" : ""}>
                  ${escapeHTML(project.DisplayName)}
                </option>
              `).join("")}
            </select>
            <div class="path-picker-row">
              <p>${escapeHTML(state.project?.RootPath || "~/.ergo-loom")}</p>
              <button type="button" class="path-picker-button">경로 선택</button>
            </div>
          </div>
          <div class="new-chat-setup-block">
            <h3>AI Provider</h3>
            <div class="provider-checks">
              ${providerOptions.length > 0 ? providerOptions.map((item, index) => `
                <label class="context-check">
                  <input type="checkbox" value="${escapeHTML(item.groupID)}" ${state.selectedProviderIds.includes(item.groupID) || index === 0 ? "checked" : ""}>
                  <span>
                    <strong>${escapeHTML(providerLabel(item.providerID))}${moderatorTagMarkup(moderator, item.groupID)}</strong>
                    <small>${item.readyModels} ready · ${item.models.length} model${item.models.length === 1 ? "" : "s"}</small>
                  </span>
                </label>
              `).join("") : `<p>사용 가능한 provider가 없어요. 좌측 AI Usage에서 provider를 연결해주세요.</p>`}
            </div>
          </div>
          <div class="new-chat-setup-block moderator-setup-block">
            <h3>Moderator</h3>
            <div class="moderator-controls">
              <label>
                <span>Mode</span>
                <select class="moderator-mode">
                  <option value="auto" ${moderator.mode === "auto" ? "selected" : ""}>Auto · registered order</option>
                  <option value="manual" ${moderator.mode === "manual" ? "selected" : ""}>Manual</option>
                </select>
              </label>
              <label>
                <span>1순위</span>
                <select class="moderator-primary" ${moderator.mode === "auto" ? "disabled" : ""}>
                  ${moderatorProviderOptions(providerOptions, moderator.primary)}
                </select>
              </label>
              <label>
                <span>2순위</span>
                <select class="moderator-secondary" ${moderator.mode === "auto" ? "disabled" : ""}>
                  ${moderatorProviderOptions(providerOptions, moderator.secondary, true)}
                </select>
              </label>
            </div>
            <p>${escapeHTML(moderatorHint(moderator))}</p>
          </div>
        </div>
      </section>
      <section class="new-chat-context">
        <section>
          <div>
            <strong>지침</strong>
            <span>＋</span>
          </div>
          <p>Ergo Loom의 응답을 맞춤화하는 지침 추가</p>
        </section>
        <section>
          <div>
            <strong>파일</strong>
            <span>＋</span>
          </div>
          <div class="new-chat-file-drop">
            <span>문서, 로그, 코드 파일을 이 프로젝트 컨텍스트로 추가하세요.</span>
          </div>
        </section>
      </section>
    </div>
  `;
  els.messages.append(home);
  home.querySelector(".project-context-select")?.addEventListener("change", (event) => {
    const projectID = (event.target as HTMLSelectElement).value;
    selectProject(projectID);
  });
  home.querySelector(".path-picker-button")?.addEventListener("click", chooseProjectPath);
  home.querySelectorAll(".moderator-mode, .moderator-primary, .moderator-secondary").forEach((input) => {
    input.addEventListener("change", () => saveModeratorFromHome(home));
  });
  home.querySelectorAll(".provider-checks input[type='checkbox']").forEach((input) => {
    input.addEventListener("change", () => {
      const selected = [...home.querySelectorAll(".provider-checks input[type='checkbox']:checked")]
        .map((node) => (node as HTMLInputElement).value);
      state.selectedProviderIds = selected;
      saveSelectedProviderIDs(selected);
      renderModelPicker();
    });
  });
}

function randomWarmBuildPrompt() {
  return warmBuildPrompts[Math.floor(Math.random() * warmBuildPrompts.length)];
}

function moderatorTagMarkup(moderator, groupID) {
  if (moderator.primary === groupID) {
    return ` <em class="moderator-tag">moderator<span class="moderator-help" tabindex="0" aria-label="Moderator provider tooltip">?</span><span class="moderator-tooltip" role="tooltip">여러 AI provider가 같은 채팅에서 함께 답할 때 흐름과 순서를 중재하는 provider입니다.</span></em>`;
  }
  if (moderator.secondary === groupID) {
    return ` <em class="moderator-tag secondary">secondary<span class="moderator-help" tabindex="0" aria-label="Secondary moderator tooltip">?</span><span class="moderator-tooltip" role="tooltip">Primary moderator에 장애가 생기면 이어받는 보조 moderator provider입니다.</span></em>`;
  }
  return "";
}

function effectiveModerator(providerOptions = availableProviderOptions()) {
  const pref = state.moderatorPreference || {};
  const mode = pref.Mode === "manual" ? "manual" : "auto";
  const ordered = providerOptions.map((item) => item.groupID);
  const primary = mode === "manual" && ordered.includes(pref.PrimaryProviderGroupID)
    ? pref.PrimaryProviderGroupID
    : ordered[0] || "";
  const secondary = mode === "manual" && ordered.includes(pref.SecondaryProviderGroupID) && pref.SecondaryProviderGroupID !== primary
    ? pref.SecondaryProviderGroupID
    : ordered.find((id) => id !== primary) || "";
  return {
    mode,
    source: pref.Source || "default",
    primary,
    secondary,
  };
}

function moderatorProviderOptions(providerOptions, selected, includeNone = false) {
  const rows = [];
  if (includeNone) {
    rows.push(`<option value="" ${selected ? "" : "selected"}>None</option>`);
  }
  for (const item of providerOptions) {
    rows.push(`<option value="${escapeHTML(item.groupID)}" ${item.groupID === selected ? "selected" : ""}>${escapeHTML(providerGroupLabel(item.groupID))}</option>`);
  }
  return rows.join("");
}

function moderatorHint(moderator) {
  const primary = moderator.primary ? providerGroupLabel(moderator.primary) : "none";
  const secondary = moderator.secondary ? providerGroupLabel(moderator.secondary) : "none";
  if (moderator.mode === "auto") {
    return `Auto: ${primary} 이후 장애 시 ${secondary} 순서로 중재합니다.`;
  }
  return `Manual: ${primary}를 우선 사용하고, 장애 시 ${secondary}로 전환합니다.`;
}

async function saveModeratorFromHome(home) {
  if (!state.project?.ID) return;
  const mode = (home.querySelector(".moderator-mode") as HTMLSelectElement)?.value || "auto";
  const primary = (home.querySelector(".moderator-primary") as HTMLSelectElement)?.value || "";
  const secondary = (home.querySelector(".moderator-secondary") as HTMLSelectElement)?.value || "";
  const data = await request(`/api/projects/${encodeURIComponent(state.project.ID)}/moderator`, {
    method: "POST",
    body: JSON.stringify({
      mode,
      primaryProviderGroupId: primary,
      secondaryProviderGroupId: secondary,
    }),
  });
  state.moderatorPreference = data.moderatorPreference || state.moderatorPreference;
  renderEmptyChat();
}

async function chooseProjectPath() {
  let selectedPath = "";
  const bridge = (window as any).ergoLoom;
  const isPackagedLikeDesktop = new URLSearchParams(window.location.search).get("desktop") === "1";
  if (bridge?.chooseDirectory) {
    try {
      const result = await bridge.chooseDirectory();
      if (result?.canceled) return;
      selectedPath = String(result?.path || "").trim();
    } catch (error) {
      const fallback = String(await appPrompt({
        title: "프로젝트 경로 선택",
        message: `파일 탐색기를 열지 못했습니다.\n${error?.message || String(error)}\n경로를 직접 입력하면 해당 경로의 프로젝트로 전환됩니다.`,
        value: state.project?.RootPath || "~/.ergo-loom",
        placeholder: "/Users/name/project",
        confirmLabel: "Use path",
      }) || "").trim();
      selectedPath = fallback;
    }
  } else {
    selectedPath = String(await appPrompt({
      title: "프로젝트 경로 선택",
      message: isPackagedLikeDesktop
        ? "설치 앱의 파일 선택 브리지를 찾지 못했습니다. 경로를 직접 입력하면 해당 경로의 프로젝트로 전환됩니다."
        : "브라우저 개발 모드에서는 파일 탐색기 대신 경로를 직접 입력합니다. 경로가 바뀌면 현재 프로젝트 수정이 아니라 해당 경로의 프로젝트로 전환됩니다.",
      value: state.project?.RootPath || "~/.ergo-loom",
      placeholder: "/Users/name/project",
      confirmLabel: "Use path",
    }) || "").trim();
  }
  if (!selectedPath) return;

  const existing = state.projects.find((project) => normalizePath(project.RootPath) === normalizePath(selectedPath));
  if (existing) {
    await selectProject(existing.ID);
    return;
  }
  const displayName = projectNameFromPath(selectedPath);
  const data = await request("/api/projects", {
    method: "POST",
    body: JSON.stringify({ displayName, rootPath: selectedPath }),
  });
  await selectProject(data.project.ID);
}

function normalizePath(value) {
  return String(value || "").trim().replace(/\/+$/, "");
}

function projectNameFromPath(value) {
  const normalized = normalizePath(value);
  const name = normalized.split("/").filter(Boolean).pop();
  return name || "Local project";
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
    const tabButton = button as HTMLElement;
    tabButton.classList.toggle("active", tabButton.dataset.workspaceTab === tabID);
  }
  for (const panel of document.querySelectorAll("[data-workspace-panel]")) {
    const workspacePanel = panel as HTMLElement;
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
  if (!filePath) return;
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
  } catch (error) {
    tab.content = error.message || String(error);
    tab.status = "error";
    tab.size = 0;
    appendWorkspaceEvent("error", { toolName: "file.read", command: filePath, text: tab.content });
  }
  renderFilePanel();
}

function reloadActiveFile() {
  const tab = activeFileTab();
  if (!tab.path) return;
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
  const providerRows = usageRowsForProviders(connectedProviderIDsForUsage(items));
  renderAIUsageList(els.usage, providerRows);
}

function renderNavUsage() {
  els.navUsage.replaceChildren();
  const rows = usageRowsForProviders(connectedProviderIDsForUsage());
  if (rows.length === 0) {
    const empty = document.createElement("div");
    empty.className = "meta";
    empty.textContent = "No provider connected";
    els.navUsage.append(empty);
    els.usageGaugeFill.style.width = "0%";
    return;
  }

  const totalUsed = rows.reduce((sum, item) => sum + item.used, 0);
  const softCap = Math.max(50000, rows.length * 50000);
  const remaining = Math.max(0, softCap - totalUsed);
  els.usageGaugeFill.style.width = `${Math.min(100, Math.round((remaining / softCap) * 100))}%`;

  renderAIUsageList(els.navUsage, rows);
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
        <button class="ai-connect-button" type="button" data-provider-id="${escapeHTML(item.providerID)}" data-account-label="${escapeHTML(item.rawAccountLabel)}">${item.connected ? "Edit" : "Connect"}</button>
      </div>
    `;
    list.append(row);
  }
  target.append(list);
  for (const button of target.querySelectorAll(".ai-connect-button")) {
    button.addEventListener("click", () => connectProvider(button.dataset.providerId, button.dataset.accountLabel || ""));
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
    const auth = providerIDsInGroup.map(authForProvider).find((item) => item?.connected) || null;
    const profileLabels = profiles.map((profile) => profile.DisplayName).filter((label) => !isGenericAccountLabel(label));
    const rawAccountLabel = profileLabels.length > 0 ? profileLabels.join(" / ") : auth?.accountLabel || profiles[0]?.DisplayName || "";
    const remaining = Math.max(0, providerCap - used);
    return {
      providerID: providerIDsInGroup[0],
      providerName: providerGroupLabel(groupID),
      connected: providerIDsInGroup.some(providerIsConnected),
      used,
      rawAccountLabel,
      accountLabel: rawAccountLabel ? `account: ${rawAccountLabel}` : "account not connected",
      remainingLabel: remainingUsageLabel(route, used, providerCap),
      percent: Math.min(100, Math.round((remaining / providerCap) * 100)),
      color: providerColor(providerIDsInGroup[0]),
    };
  });
}

async function connectProvider(providerID, currentLabel = "") {
  const label = providerLabel(providerID);
  const normalizedCurrentLabel = String(currentLabel || "").trim();
  try {
    if (normalizedCurrentLabel) {
      const manualName = String(await appPrompt({
        title: `${label} account label`,
        value: normalizedCurrentLabel,
        placeholder: "Email or nickname",
        confirmLabel: "Save",
      }) || "").trim();
      if (!manualName) return;
      await request("/api/provider-profiles/connect", {
        method: "POST",
        body: JSON.stringify({ providerPluginId: providerID, displayName: manualName }),
      });
      await addProviderRoute(providerID);
      await loadState();
      return;
    }
    const data = await request("/api/provider-profiles/connect", {
      method: "POST",
      body: JSON.stringify({ providerPluginId: providerID }),
    });
    await addProviderRoute(providerID);
    await loadState();
    const account = data.profile?.DisplayName || label;
    appendActivityEvent("result", { toolName: "auth.connect", command: label, text: `Connected account: ${account}` });
  } catch (error) {
    if (providerID === "anthropic") {
      await appAlert({
        title: "Claude subscription login required",
        message: "Claude는 claude.ai 웹 로그인 대신 Claude CLI subscription token으로 연결해야 합니다.\n\n터미널에서 `claude setup-token`을 실행해 Pro/Max 구독 계정을 연결한 뒤, AI Usage의 refresh 버튼을 눌러주세요.",
        confirmLabel: "OK",
      });
      appendActivityEvent("error", { toolName: "auth.connect", command: label, text: error?.message || String(error) });
      return;
    }
    const message = error?.message || `${label} account bridge is not available`;
    const manualName = String(await appPrompt({
      title: `${label} account label`,
      message: `자동 탐지: ${message}\n현재 로컬 라이선스/웹 계정으로 사용할 닉네임이나 이메일을 입력하세요.`,
      value: `${label} account`,
      placeholder: "Email or nickname",
      confirmLabel: "Save",
    }) || "").trim();
    if (!manualName) return;
    await request("/api/provider-profiles/connect", {
      method: "POST",
      body: JSON.stringify({ providerPluginId: providerID, displayName: manualName }),
    });
    await addProviderRoute(providerID);
    await loadState();
  }
}

async function openClaudeHandoffSetup() {
  const bridge = (window as any).ergoLoom;
  if (!bridge?.openClaudeWorker) return;
  await bridge.openClaudeWorker();
}

function allProviderIDsForUsage(items) {
  const ids = new Set(state.routes.map((route) => route.ProviderPluginID));
  for (const profile of state.profiles) ids.add(profile.ProviderPluginID);
  for (const item of state.projectRoutes.filter((item) => item.Enabled)) ids.add(item.Route.ProviderPluginID);
  for (const item of items) ids.add(item.ProviderPluginID);
  return [...ids];
}

function connectedProviderIDsForUsage(items = []) {
  const ids = new Set();
  for (const profile of state.profiles) {
    if (providerIsConnected(profile.ProviderPluginID)) ids.add(profile.ProviderPluginID);
  }
  for (const item of state.projectRoutes.filter((item) => item.Enabled)) {
    if (providerIsConnected(item.Route.ProviderPluginID)) {
      ids.add(item.Route.ProviderPluginID);
    }
  }
  for (const item of state.usage) {
    if (item.ProviderPluginID && providerIsConnected(item.ProviderPluginID)) {
      ids.add(item.ProviderPluginID);
    }
  }
  for (const item of items) {
    if (item.ProviderPluginID && providerIsConnected(item.ProviderPluginID)) {
      ids.add(item.ProviderPluginID);
    }
  }
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

function authForProvider(providerID) {
  const authID = {
    anthropic: "claude",
    codex: "codex",
    copilot: "copilot",
    gemini: "gemini",
    ollama: "ollama",
    openai: "codex",
  }[providerID] || providerID;
  return state.authStatuses.find((item) => item.id === authID) || null;
}

function providerIsConnected(providerID) {
  const auth = authForProvider(providerID);
  if (auth?.connected) return true;
  if (auth) return false;
  if (providerRequiresLiveAuth(providerID)) return false;
  return Boolean(profileForProvider(providerID));
}

function hasDesktopHandoffBridge() {
  return Boolean((window as any).ergoLoom?.handoffBridge);
}

function providerRequiresLiveAuth(providerID) {
  return ["anthropic", "codex", "copilot", "gemini", "ollama", "openai"].includes(providerID);
}

function isGenericAccountLabel(label) {
  return [
    "Codex local account",
    "GitHub Copilot account",
    "Claude local account",
    "Gemini local account",
    "Ollama local runtime",
  ].includes(String(label || "").trim());
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
    gemini: "#c08a45",
    ollama: "#6f7b5a",
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
    const actionable = badge === "Add" || badge === "Connect" || badge === "Setup";
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
    } else if (badge === "Add") {
      row.addEventListener("click", () => addModelProvider(item.model.ProviderPluginID));
    } else if (badge === "Connect" || badge === "Setup") {
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

function closeModelMenu() {
  if (els.modelMenu.hidden) return;
  els.modelMenu.hidden = true;
  els.modelPicker.setAttribute("aria-expanded", "false");
}

function closeModelMenuFromOutside(event) {
  if (els.modelMenu.hidden) return;
  if (els.modelMenu.contains(event.target) || els.modelPicker.contains(event.target)) return;
  closeModelMenu();
}

function selectModel(modelID) {
  state.selectedModelId = modelID;
  saveSelectedModelID(modelID);
  const selected = selectedModelOption();
  els.modelPickerLabel.textContent = selected ? modelDisplayName(selected.model) : "No model";
  renderActiveRouteSummary();
  closeModelMenu();
  renderModelMenu(modelOptions());
}

function modelDisplayName(model) {
  return model.DisplayName.replace(/^GPT-/, "GPT-");
}

function modelDescription(model) {
  const provider = providerLabel(model.ProviderPluginID);
  if (model.ProviderPluginID === "codex") return `${provider} · local subscription route`;
  if (model.ProviderPluginID === "anthropic") return `${provider} · CLI subscription route`;
  if (model.ProviderPluginID === "copilot") return `${provider} · SDK or VS Code bridge`;
  if (model.ProviderPluginID === "gemini") return `${provider} · CLI or web handoff`;
  if (model.ProviderPluginID === "ollama") return `${provider} · local runtime`;
  if (model.ProviderPluginID === "openai") return `${provider} · web handoff`;
  return `${provider} model`;
}

function modelOptions(options: any = {}) {
  const respectSessionProviders = options.respectSessionProviders !== false;
  const allowedGroups = respectSessionProviders ? activeProviderGroupSet() : null;
  const enabledProviders = new Map();
  for (const item of state.projectRoutes) {
    if (item.Enabled) {
      const current = enabledProviders.get(item.Route.ProviderPluginID);
      if (!current || routePreference(item.Route) < routePreference(current)) {
        enabledProviders.set(item.Route.ProviderPluginID, item.Route);
      }
    }
  }
  return state.models
    .map((model) => ({
      model,
      route: enabledProviders.get(model.ProviderPluginID) || null,
      routeId: enabledProviders.get(model.ProviderPluginID)?.ID || "",
      connected: providerIsConnected(model.ProviderPluginID) || isFreeHandoffRoute(enabledProviders.get(model.ProviderPluginID)),
      activeInSession: !allowedGroups || allowedGroups.has(providerGroupID(model.ProviderPluginID)),
    }));
}

function availableProviderOptions() {
  const groups = new Map();
  for (const routeItem of state.projectRoutes) {
    if (!routeItem.Enabled) continue;
    const route = routeItem.Route;
    const groupID = providerGroupID(route.ProviderPluginID);
    const existing = groups.get(groupID);
    if (!existing || routePreference(route) < routePreference(existing.route)) {
      groups.set(groupID, {
        groupID,
        providerID: route.ProviderPluginID,
        route,
        models: [],
        readyModels: 0,
      });
    }
  }

  for (const item of modelOptions({ respectSessionProviders: false }).sort(compareModelOptions)) {
    const groupID = providerGroupID(item.model.ProviderPluginID);
    const existing = groups.get(groupID);
    if (!existing) {
      continue;
    }
    existing.models.push(item.model);
    if (isModelSelectable(item)) {
      existing.readyModels += 1;
    }
  }

  return [...groups.values()]
    .filter((item) => item.route?.Status === "available" && (providerIsConnected(item.providerID) || isSetupRoute(item.route) || isFreeHandoffRoute(item.route) || item.route.SupportsHandoff))
    .sort((a, b) => providerGroupOrder(a.groupID) - providerGroupOrder(b.groupID));
}

function activeProviderGroupSet() {
  if (state.selectedProviderIds.length === 0) return null;
  return new Set(state.selectedProviderIds);
}

function normalizeSelectedProviders() {
  const available = availableProviderOptions();
  const availableIDs = new Set(available.map((item) => item.groupID));
  const saved = savedSelectedProviderIDs().filter((id) => availableIDs.has(id));
  if (saved.length > 0) {
    state.selectedProviderIds = saved;
    return;
  }
  const selected = selectedModelOption();
  const selectedGroup = selected ? providerGroupID(selected.model.ProviderPluginID) : "";
  state.selectedProviderIds = selectedGroup && availableIDs.has(selectedGroup)
    ? [selectedGroup]
    : available.slice(0, 1).map((item) => item.groupID);
  saveSelectedProviderIDs(state.selectedProviderIds);
}

function selectedProviderStorageKey() {
  return `ergo-loom:selected-providers:${state.project?.ID || "default"}`;
}

function savedSelectedProviderIDs() {
  const raw = window.localStorage.getItem(selectedProviderStorageKey()) || "";
  return raw.split(",").map((item) => item.trim()).filter(Boolean);
}

function saveSelectedProviderIDs(providerIDs) {
  window.localStorage.setItem(selectedProviderStorageKey(), providerIDs.filter(Boolean).join(","));
}

function routePreference(route) {
  if (!route) return 999;
  if (route.ProviderPluginID === "anthropic") {
    if (route.Transport === "claude_cli") return 0;
    if (route.Transport === "manual") return 2;
  }
  if (isExecutableRoute(route) && providerIsConnected(route.ProviderPluginID)) return 0;
  if (isFreeHandoffRoute(route)) return 1;
  if (route.SupportsHandoff) return 2;
  return 3;
}

function compareModelOptions(a, b) {
  const groupCompare = providerGroupOrder(a.model.ProviderPluginID) - providerGroupOrder(b.model.ProviderPluginID);
  if (groupCompare !== 0) return groupCompare;
  const providerCompare = providerOrder(a.model.ProviderPluginID) - providerOrder(b.model.ProviderPluginID);
  if (providerCompare !== 0) return providerCompare;
  if (a.model.IsDefault !== b.model.IsDefault) return a.model.IsDefault ? -1 : 1;
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
  if (!modelID) return;
  window.localStorage.setItem(selectedModelStorageKey(), modelID);
}

function isModelSelectable(item) {
  if (!item.activeInSession) return false;
  if (!item.routeId || item.route?.Status !== "available") return false;
  if (item.model.Status !== "available") return false;
  return (item.connected && isExecutableRoute(item.route)) || isFreeHandoffRoute(item.route);
}

function modelBadge(item) {
  if (!item.activeInSession) return "Add";
  if (!item.routeId) return "Add";
  if (item.route?.Status !== "available") return "Planned";
  if (item.model.Status === "upgrade") return "Upgrade";
  if (isFreeHandoffRoute(item.route)) return "Handoff";
  if (!item.connected && isSetupRoute(item.route)) return "Setup";
  if (!item.connected) return "Connect";
  if (item.model.Status === "available") return isExecutableRoute(item.route) ? "" : "Not ready";
  if (item.model.Status === "handoff") return item.connected ? "Not ready" : "Connect";
  if (item.model.Status === "bridge_required") return item.connected ? "Not ready" : "Connect";
  return "Planned";
}

function isExecutableRoute(route) {
  return (route?.ProviderPluginID === "codex" && route?.Transport === "app_server")
    || (route?.ProviderPluginID === "anthropic" && route?.Transport === "claude_cli");
}

function isFreeHandoffRoute(route) {
  if (route?.ProviderPluginID === "anthropic") return false;
  if (!(route?.Status === "available" && route?.SupportsHandoff && route?.AccessMode === "free_handoff")) return false;
  return true;
}

function isSetupRoute(route) {
  return route?.ProviderPluginID === "anthropic" && route?.Transport === "claude_cli";
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
    if (!item.Enabled || seen.has(providerID)) continue;
    seen.add(providerID);
    rows.push(item);
  }
  return rows;
}

function modelLabelForIds(modelId: string, _routeId: string): string {
  const model = state.models.find((m) => m.ID === modelId);
  if (!model) return modelId || "Unknown model";
  return `${providerLabel(model.ProviderPluginID)} · ${modelDisplayName(model)}`;
}

function providerLabel(providerID) {
  const names = {
    anthropic: "Claude",
    codex: "Codex/ChatGPT",
    copilot: "VSCode Copilot",
    gemini: "Gemini",
    ollama: "Ollama(Local Model)",
    openai: "Codex/ChatGPT",
  };
  return names[providerID] || providerID;
}

function providerGroupID(providerID) {
  if (providerID === "codex" || providerID === "openai" || providerID === "codex/openai" || providerID === "codex-openai") return "codex-openai";
  return providerID;
}

function providerGroupLabel(groupID) {
  if (groupID === "codex-openai") return "Codex/ChatGPT";
  return providerLabel(groupID);
}

function providerIDsForGroup(groupID) {
  if (groupID === "codex-openai") return ["codex", "openai"];
  return [groupID];
}

function uniqueProviderGroups(providerIDs) {
  const seen = new Set();
  const groups = [];
  for (const providerID of providerIDs) {
    const groupID = providerGroupID(providerID);
    if (seen.has(groupID)) continue;
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
    gemini: 40,
    ollama: 50,
  };
  return order[providerGroupID(providerID)] || 999;
}

function providerOrder(providerID) {
  const order = {
    codex: 10,
    openai: 20,
    anthropic: 30,
    copilot: 40,
    gemini: 50,
    ollama: 60,
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
  if (costModel.includes("license")) return "license quota";
  if (costModel.includes("free")) return "free quota";
  if (costModel.includes("local")) return "local compute";
  return "tracked quota";
}

function licenseLabel(route) {
  if (route.RequiresLicense) return `license: required · ${route.AccessMode}`;
  if (route.AccessMode.includes("free")) return `license: free tier · ${route.AccessMode}`;
  if (route.AccessMode === "local") return "license: local runtime";
  return `license: not required · ${route.AccessMode}`;
}

function accountLabel(profile) {
  if (!profile) return "account: not connected";
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
  if (!value) return "";
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
els.deleteProject.addEventListener("click", deleteCurrentProject);
els.openSearch.addEventListener("click", openSearchModal);
els.searchBackdrop.addEventListener("click", closeSearchModal);
els.sessionSearch.addEventListener("input", searchSessions);
els.sessionSearch.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closeSearchModal();
});
els.activeRoute.addEventListener("click", toggleProjectRoutes);
els.usageToggle.addEventListener("click", toggleAIUsage);
els.providerRefresh.addEventListener("click", () => refreshProviderStatus());
els.reasoningToggle.addEventListener("click", toggleReasoning);
els.addRoute.addEventListener("click", addProjectRoute);
els.composer.addEventListener("submit", sendMessage);
els.input.addEventListener("keydown", handleComposerKeydown);
els.modelPicker.addEventListener("click", toggleModelMenu);
document.addEventListener("pointerdown", closeModelMenuFromOutside);
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closeModelMenu();
});
els.modelEffort.addEventListener("click", toggleThinkingEffort);
els.modelSearch.addEventListener("input", () => renderModelMenu(modelOptions()));
els.workspaceTabs.addEventListener("click", (event) => {
  const button = event.target.closest("[data-workspace-tab]");
  if (!button) return;
  switchWorkspaceTab(button.dataset.workspaceTab);
});
els.newTerminalTab.addEventListener("click", newTerminalTab);
els.terminalCwd.addEventListener("change", () => {
  activeTerminalTab().workingDir = els.terminalCwd.value.trim();
});
els.terminalHistory.addEventListener("change", () => {
  if (els.terminalHistory.value) els.terminalCommand.value = els.terminalHistory.value;
});
els.terminalStop.addEventListener("click", stopTerminalCommand);
els.terminalForm.addEventListener("submit", queueTerminalCommand);
els.terminalRun.addEventListener("click", runTerminalCommand);
els.terminalCancel.addEventListener("click", cancelTerminalCommand);
els.newFileTab.addEventListener("click", newFileTab);
els.fileOpenForm.addEventListener("submit", openFileInActiveTab);
els.fileRecent.addEventListener("change", () => {
  if (els.fileRecent.value) openFilePath(els.fileRecent.value);
});
els.fileReload.addEventListener("click", reloadActiveFile);
switchWorkspaceTab(state.activeWorkspaceTab);
renderTerminalPanel();
renderFilePanel();
installDesktopWindowChrome();
loadState().catch((error) => {
  els.messages.textContent = error.message;
});
