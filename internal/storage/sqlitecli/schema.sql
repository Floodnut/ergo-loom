PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS source_tools (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL
);

INSERT OR IGNORE INTO source_tools (id, display_name) VALUES
  ('ergo', 'Ergo Loom'),
  ('codex', 'Codex'),
  ('copilot', 'VSCode Copilot'),
  ('claude', 'Claude Code'),
  ('gemini', 'Gemini CLI');

CREATE TABLE IF NOT EXISTS provider_plugins (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  kind TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);

INSERT OR IGNORE INTO provider_plugins (id, display_name, kind) VALUES
  ('codex', 'Codex', 'remote-ai'),
  ('openai', 'OpenAI', 'remote-ai'),
  ('anthropic', 'Anthropic', 'remote-ai'),
  ('gemini', 'Gemini', 'remote-ai'),
  ('copilot', 'VSCode Copilot', 'remote-ai'),
  ('ollama', 'Ollama', 'local-ai');

CREATE TABLE IF NOT EXISTS provider_profiles (
  id TEXT PRIMARY KEY,
  provider_plugin_id TEXT NOT NULL REFERENCES provider_plugins(id),
  display_name TEXT NOT NULL,
  credential_ref TEXT,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS provider_models (
  id TEXT PRIMARY KEY,
  provider_plugin_id TEXT NOT NULL REFERENCES provider_plugins(id),
  display_name TEXT NOT NULL,
  model_ref TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'available',
  is_default INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO provider_models (id, provider_plugin_id, display_name, model_ref, status, is_default) VALUES
  ('codex-gpt-5-5', 'codex', 'GPT-5.5', 'gpt-5.5', 'available', 1),
  ('codex-gpt-5-5-fast', 'codex', 'GPT-5.5 Fast', 'gpt-5.5-fast', 'available', 0),
  ('codex-gpt-5-5-high', 'codex', 'GPT-5.5 High', 'gpt-5.5-high', 'available', 0),
  ('openai-chatgpt-gpt-5-5', 'openai', 'GPT-5.5', 'gpt-5.5', 'handoff', 1),
  ('openai-chatgpt-gpt-5-5-thinking', 'openai', 'GPT-5.5 Thinking', 'gpt-5.5-thinking', 'handoff', 0),
  ('openai-chatgpt-gpt-5-3-fast', 'openai', 'GPT-5.3 Fast', 'gpt-5.3-fast', 'handoff', 0),
  ('anthropic-claude-sonnet-4-6', 'anthropic', 'Claude Sonnet 4.6', 'claude-sonnet-4.6', 'available', 1),
  ('anthropic-claude-opus-4-8', 'anthropic', 'Claude Opus 4.8', 'claude-opus-4.8', 'upgrade', 0),
  ('anthropic-claude-haiku-4-5', 'anthropic', 'Claude Haiku 4.5', 'claude-haiku-4.5', 'available', 0),
  ('anthropic-claude-opus-4-7', 'anthropic', 'Claude Opus 4.7', 'claude-opus-4.7', 'upgrade', 0),
  ('anthropic-claude-opus-4-6', 'anthropic', 'Claude Opus 4.6', 'claude-opus-4.6', 'upgrade', 0),
  ('anthropic-claude-opus-3', 'anthropic', 'Claude Opus 3', 'claude-opus-3', 'upgrade', 0),
  ('gemini-cli-default', 'gemini', 'Gemini CLI default', 'gemini-cli-default', 'planned', 1),
  ('gemini-2-5-pro', 'gemini', 'Gemini 2.5 Pro', 'gemini-2.5-pro', 'planned', 0),
  ('gemini-2-5-flash', 'gemini', 'Gemini 2.5 Flash', 'gemini-2.5-flash', 'planned', 0),
  ('copilot-default', 'copilot', 'Copilot default', 'copilot-default', 'bridge_required', 1),
  ('copilot-mai-code-1-flash', 'copilot', 'MAI-Code-1-Flash', 'mai-code-1-flash', 'bridge_required', 0),
  ('copilot-gpt-5-5', 'copilot', 'Copilot GPT-5.5', 'gpt-5.5', 'bridge_required', 0),
  ('copilot-claude-sonnet-4-6', 'copilot', 'Copilot Claude Sonnet 4.6', 'claude-sonnet-4.6', 'bridge_required', 0),
  ('copilot-gemini-2-5-pro', 'copilot', 'Copilot Gemini 2.5 Pro', 'gemini-2.5-pro', 'bridge_required', 0),
  ('ollama-default', 'ollama', 'Ollama default', 'ollama-default', 'planned', 1),
  ('ollama-llama3-2', 'ollama', 'Llama 3.2', 'llama3.2', 'planned', 0),
  ('ollama-qwen2-5-coder', 'ollama', 'Qwen2.5 Coder', 'qwen2.5-coder', 'planned', 0);

CREATE TABLE IF NOT EXISTS access_routes (
  id TEXT PRIMARY KEY,
  provider_plugin_id TEXT NOT NULL REFERENCES provider_plugins(id),
  display_name TEXT NOT NULL,
  access_mode TEXT NOT NULL,
  transport TEXT NOT NULL,
  requires_license INTEGER NOT NULL DEFAULT 0,
  requires_api_key INTEGER NOT NULL DEFAULT 0,
  supports_streaming INTEGER NOT NULL DEFAULT 0,
  supports_tools INTEGER NOT NULL DEFAULT 0,
  supports_import INTEGER NOT NULL DEFAULT 0,
  supports_handoff INTEGER NOT NULL DEFAULT 0,
  cost_model TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'available'
);

INSERT OR IGNORE INTO access_routes (
  id, provider_plugin_id, display_name, access_mode, transport,
  requires_license, requires_api_key, supports_streaming, supports_tools,
  supports_import, supports_handoff, cost_model, status
) VALUES
  ('codex-subscription-cli', 'codex', 'Codex via local CLI or app-server', 'subscription_native', 'cli_or_app_server', 1, 0, 1, 1, 1, 0, 'included_with_license_limits', 'planned'),
  ('chatgpt-web-handoff', 'openai', 'ChatGPT web handoff', 'licensed_handoff', 'manual', 0, 0, 0, 0, 1, 1, 'included_with_chat_subscription_or_free_limits', 'planned'),
  ('claude-code-cli', 'anthropic', 'Claude Code CLI', 'subscription_native', 'claude_cli', 1, 0, 1, 0, 1, 0, 'included_with_license_or_free_limits', 'available'),
  ('claude-sdk-bridge', 'anthropic', 'Claude Agent SDK bridge', 'subscription_native', 'claude_sdk_bridge', 1, 0, 1, 0, 0, 0, 'included_with_license_or_free_limits', 'available'),
  ('claude-web-free-handoff', 'anthropic', 'Claude web free handoff', 'free_handoff', 'manual', 0, 0, 0, 0, 1, 1, 'included_with_free_limits', 'planned'),
  ('claude-web-licensed-handoff', 'anthropic', 'Claude web licensed handoff', 'licensed_handoff', 'manual', 1, 0, 0, 0, 1, 1, 'included_with_license_limits', 'planned'),
  ('copilot-vscode-bridge', 'copilot', 'VS Code Copilot bridge', 'subscription_native', 'ide_bridge', 1, 0, 1, 1, 0, 0, 'included_with_license_limits', 'planned'),
  ('copilot-sdk-cli', 'copilot', 'GitHub Copilot SDK / CLI server', 'subscription_native', 'copilot_sdk_jsonrpc', 1, 0, 1, 1, 0, 0, 'included_with_copilot_premium_requests', 'planned'),
  ('gemini-cli-free', 'gemini', 'Gemini CLI personal account', 'free_native', 'cli', 0, 0, 1, 1, 1, 0, 'included_with_free_limits', 'planned'),
  ('gemini-cli-code-assist', 'gemini', 'Gemini CLI Code Assist', 'subscription_native', 'cli', 1, 0, 1, 1, 1, 0, 'included_with_license_limits', 'planned'),
  ('gemini-web-handoff', 'gemini', 'Gemini web handoff', 'free_handoff', 'manual', 0, 0, 0, 0, 1, 1, 'included_with_free_or_license_limits', 'planned'),
  ('ollama-local', 'ollama', 'Ollama local runtime', 'local', 'ollama_http', 0, 0, 1, 0, 0, 0, 'local_compute', 'planned');

UPDATE access_routes
SET display_name = 'Codex SDK/app-server',
    transport = 'app_server',
    status = 'available'
WHERE id = 'codex-subscription-cli';

UPDATE access_routes
SET status = 'available'
WHERE id IN (
  'chatgpt-web-handoff',
  'copilot-sdk-cli',
  'copilot-vscode-bridge',
  'gemini-web-handoff'
);

UPDATE access_routes
SET status = 'planned'
WHERE id IN ('claude-web-free-handoff', 'claude-web-licensed-handoff');

UPDATE provider_models
SET status = 'bridge_required'
WHERE id = 'copilot-default';

UPDATE provider_models
SET status = 'available'
WHERE id IN ('anthropic-claude-sonnet-4-6', 'anthropic-claude-haiku-4-5');

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  root_path TEXT,
  is_default INTEGER NOT NULL DEFAULT 0,
  context_policy TEXT NOT NULL DEFAULT 'flat-trim',
  handoff_policy TEXT NOT NULL DEFAULT 'route-change',
  route_policy TEXT NOT NULL DEFAULT 'manual',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT OR IGNORE INTO projects (id, display_name, root_path, is_default) VALUES
  ('default', 'Default Project', NULL, 1);

CREATE TABLE IF NOT EXISTS project_access_routes (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  access_route_id TEXT NOT NULL REFERENCES access_routes(id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 1,
  priority INTEGER NOT NULL DEFAULT 100,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY (project_id, access_route_id)
);

CREATE TABLE IF NOT EXISTS moderator_preferences (
  scope TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'auto',
  primary_provider_group_id TEXT,
  secondary_provider_group_id TEXT,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY (scope, project_id)
);

INSERT OR IGNORE INTO moderator_preferences (scope, project_id, mode) VALUES
  ('global', '', 'auto');

CREATE TABLE IF NOT EXISTS command_runs (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
  session_id TEXT REFERENCES sessions(id) ON DELETE SET NULL,
  command TEXT NOT NULL,
  working_dir TEXT NOT NULL,
  status TEXT NOT NULL,
  exit_code INTEGER,
  stdout TEXT NOT NULL DEFAULT '',
  stderr TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  finished_at TEXT
);

-- Claude fallback order within 'anthropic' group: sdk(15) → cli(20) → handoff(30/35)
-- sdk and cli require a paid subscription; handoff is the free/licensed fallback.
INSERT OR IGNORE INTO project_access_routes (project_id, access_route_id, enabled, priority) VALUES
  ('default', 'codex-subscription-cli',      1, 10),
  ('default', 'claude-sdk-bridge',            1, 15),
  ('default', 'claude-code-cli',              1, 20),
  ('default', 'claude-web-free-handoff',      1, 30),
  ('default', 'claude-web-licensed-handoff',  1, 35),
  ('default', 'copilot-sdk-cli',              1, 50),
  ('default', 'copilot-vscode-bridge',        1, 60);

CREATE TABLE IF NOT EXISTS provider_chats (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  provider_plugin_id TEXT NOT NULL REFERENCES provider_plugins(id),
  provider_profile_id TEXT REFERENCES provider_profiles(id),
  access_route_id TEXT REFERENCES access_routes(id),
  model_id TEXT REFERENCES provider_models(id),
  external_thread_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (session_id, provider_plugin_id, provider_profile_id, access_route_id, model_id)
);

CREATE TABLE IF NOT EXISTS tool_registry (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  kind TEXT NOT NULL,
  transport TEXT NOT NULL,
  requires_approval INTEGER NOT NULL DEFAULT 1,
  enabled INTEGER NOT NULL DEFAULT 1
);

INSERT OR IGNORE INTO tool_registry (id, display_name, kind, transport, requires_approval) VALUES
  ('shell.command', 'Shell command', 'shell', 'local_process', 1),
  ('http.request', 'HTTP request', 'network', 'local_process', 1),
  ('openstack.cli', 'OpenStack CLI', 'cloud', 'local_process', 1),
  ('kubectl.command', 'kubectl', 'cluster', 'local_process', 1),
  ('docker.command', 'Docker', 'container', 'local_process', 1);

CREATE TABLE IF NOT EXISTS agent_plugins (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  uses_ai INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);

INSERT OR IGNORE INTO agent_plugins (id, display_name, uses_ai) VALUES
  ('chat', 'Chat', 1),
  ('review', 'Code Review', 1),
  ('summarize', 'Summarize', 1),
  ('merge-context', 'Merge Context', 1),
  ('import-session', 'Import Session', 0),
  ('search-kb', 'Search Knowledge Base', 0);

CREATE TABLE IF NOT EXISTS capabilities (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  kind TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);

INSERT OR IGNORE INTO capabilities (id, display_name, kind) VALUES
  ('sqlite-storage', 'SQLite Storage', 'local'),
  ('session-import', 'Session Import', 'local'),
  ('file-index', 'File Index', 'local'),
  ('keyword-search', 'Keyword Search', 'local'),
  ('git-inspect', 'Git Inspect', 'local'),
  ('http-request', 'HTTP Request', 'local'),
  ('export', 'Export', 'local');

CREATE TABLE IF NOT EXISTS raw_imports (
  id TEXT PRIMARY KEY,
  source_tool TEXT NOT NULL REFERENCES source_tools(id),
  source_id TEXT NOT NULL,
  payload_path TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL,
  imported_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (source_tool, source_id, payload_sha256)
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id),
  source_tool TEXT NOT NULL REFERENCES source_tools(id),
  source_id TEXT NOT NULL,
  raw_import_id TEXT REFERENCES raw_imports(id),
  title TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  parent_session_id TEXT REFERENCES sessions(id),
  branch_from_message_id TEXT,
  UNIQUE (source_tool, source_id)
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  source_id TEXT,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  UNIQUE (session_id, ordinal)
);

CREATE TABLE IF NOT EXISTS message_events (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
  activity_index INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_message_events_session
ON message_events(session_id, activity_index, created_at);

CREATE TABLE IF NOT EXISTS context_events (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
  session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  branch_id TEXT,
  payload_ref TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS context_event_parents (
  event_id TEXT NOT NULL REFERENCES context_events(id) ON DELETE CASCADE,
  parent_event_id TEXT NOT NULL REFERENCES context_events(id) ON DELETE CASCADE,
  ordinal INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (event_id, parent_event_id)
);

CREATE TABLE IF NOT EXISTS context_branches (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  from_event_id TEXT NOT NULL REFERENCES context_events(id) ON DELETE CASCADE,
  head_event_id TEXT NOT NULL REFERENCES context_events(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (session_id, id)
);

CREATE TABLE IF NOT EXISTS context_heads (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL DEFAULT 'main',
  event_id TEXT NOT NULL REFERENCES context_events(id) ON DELETE CASCADE,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (project_id, session_id, branch_id)
);

CREATE INDEX IF NOT EXISTS idx_context_events_session
ON context_events(session_id, created_at);

CREATE INDEX IF NOT EXISTS idx_context_event_parents_parent
ON context_event_parents(parent_event_id);

CREATE INDEX IF NOT EXISTS idx_context_branches_session
ON context_branches(session_id, created_at);

CREATE TABLE IF NOT EXISTS chat_runs (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL DEFAULT 'main',
  role TEXT NOT NULL,
  status TEXT NOT NULL,
  input_event_id TEXT REFERENCES context_events(id) ON DELETE SET NULL,
  output_event_id TEXT REFERENCES context_events(id) ON DELETE SET NULL,
  context_packet_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS provider_segments (
  id TEXT PRIMARY KEY,
  chat_run_id TEXT NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL,
  route_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  external_thread_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  handoff_reason TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  completed_at TEXT
);

CREATE TABLE IF NOT EXISTS steering_events (
  id TEXT PRIMARY KEY,
  chat_run_id TEXT NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
  provider_segment_id TEXT REFERENCES provider_segments(id) ON DELETE SET NULL,
  event_id TEXT REFERENCES context_events(id) ON DELETE SET NULL,
  content TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'recorded',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_chat_runs_session
ON chat_runs(session_id, branch_id, created_at);

CREATE INDEX IF NOT EXISTS idx_chat_runs_status
ON chat_runs(status, role);

CREATE INDEX IF NOT EXISTS idx_chat_runs_session_active
ON chat_runs(session_id, branch_id, role, status);

CREATE INDEX IF NOT EXISTS idx_provider_segments_chat_run
ON provider_segments(chat_run_id, started_at);

CREATE INDEX IF NOT EXISTS idx_steering_events_chat_run
ON steering_events(chat_run_id, created_at);

CREATE TABLE IF NOT EXISTS chat_queue_items (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL DEFAULT 'main',
  content TEXT NOT NULL,
  mode TEXT NOT NULL DEFAULT 'normal',
  status TEXT NOT NULL DEFAULT 'pending',
  order_index INTEGER NOT NULL DEFAULT 0,
  route_id TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  thinking_effort TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_chat_queue_items_session
ON chat_queue_items(session_id, status, order_index);

CREATE TABLE IF NOT EXISTS candidate_outputs (
  id TEXT PRIMARY KEY,
  chat_run_id TEXT NOT NULL REFERENCES chat_runs(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL DEFAULT 'main',
  trigger_event_id TEXT NOT NULL DEFAULT '',
  content_ref TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_candidate_outputs_session
ON candidate_outputs(session_id, status, created_at);

CREATE TABLE IF NOT EXISTS context_packets (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL DEFAULT 'main',
  head_event_id TEXT,
  user_input TEXT NOT NULL DEFAULT '',
  content_ref TEXT NOT NULL,
  reference_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_context_packets_session
ON context_packets(session_id, created_at);

CREATE TABLE IF NOT EXISTS session_provider_groups (
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  provider_group_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY (session_id, provider_group_id)
);

CREATE TABLE IF NOT EXISTS branches (
  id TEXT PRIMARY KEY,
  parent_session_id TEXT NOT NULL REFERENCES sessions(id),
  session_id TEXT NOT NULL REFERENCES sessions(id),
  from_message_id TEXT NOT NULL REFERENCES messages(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  agent_plugin_id TEXT NOT NULL REFERENCES agent_plugins(id),
  provider_profile_id TEXT REFERENCES provider_profiles(id),
  session_id TEXT REFERENCES sessions(id),
  status TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  finished_at TEXT
);

CREATE TABLE IF NOT EXISTS token_ledger (
  id TEXT PRIMARY KEY,
  provider_plugin_id TEXT NOT NULL REFERENCES provider_plugins(id),
  provider_profile_id TEXT REFERENCES provider_profiles(id),
  agent_run_id TEXT REFERENCES agent_runs(id),
  session_id TEXT REFERENCES sessions(id),
  model TEXT NOT NULL,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  estimated_cost_cents INTEGER,
  actual_cost_cents INTEGER,
  request_id TEXT,
  status TEXT NOT NULL,
  recorded_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

DELETE FROM project_access_routes WHERE access_route_id IN ('openai-api', 'claude-agent-sdk', 'gemini-api', 'cursor-chat-handoff', 'local-model');
DELETE FROM provider_chats WHERE provider_plugin_id IN ('cursor', 'local') OR access_route_id IN ('cursor-chat-handoff', 'local-model');
DELETE FROM token_ledger WHERE provider_plugin_id IN ('cursor', 'local');
DELETE FROM provider_profiles WHERE provider_plugin_id IN ('cursor', 'local');
DELETE FROM provider_models WHERE provider_plugin_id IN ('cursor', 'local');
DELETE FROM access_routes WHERE id IN ('openai-api', 'claude-agent-sdk', 'gemini-api', 'cursor-chat-handoff', 'local-model') OR access_mode = 'api_billed';
DELETE FROM provider_plugins WHERE id = 'cursor';
DELETE FROM provider_plugins WHERE id = 'local';
DELETE FROM source_tools WHERE id = 'cursor';

CREATE TABLE IF NOT EXISTS kb_documents (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL UNIQUE,
  sha256 TEXT NOT NULL,
  indexed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS kb_terms (
  document_id TEXT NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
  term TEXT NOT NULL,
  count INTEGER NOT NULL,
  PRIMARY KEY (document_id, term)
);

CREATE TABLE IF NOT EXISTS knowledge_items (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  title TEXT NOT NULL,
  source_event_id TEXT REFERENCES context_events(id) ON DELETE SET NULL,
  content_ref TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at);
CREATE INDEX IF NOT EXISTS idx_session_provider_groups_session ON session_provider_groups(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_session_ordinal ON messages(session_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_kb_terms_term ON kb_terms(term);
CREATE INDEX IF NOT EXISTS idx_token_ledger_recorded_at ON token_ledger(recorded_at);
CREATE INDEX IF NOT EXISTS idx_token_ledger_provider_profile ON token_ledger(provider_profile_id);
CREATE INDEX IF NOT EXISTS idx_access_routes_provider ON access_routes(provider_plugin_id);
CREATE INDEX IF NOT EXISTS idx_access_routes_mode ON access_routes(access_mode);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider ON provider_models(provider_plugin_id);
CREATE INDEX IF NOT EXISTS idx_project_access_routes_project ON project_access_routes(project_id, priority);
CREATE INDEX IF NOT EXISTS idx_command_runs_started_at ON command_runs(started_at);
CREATE INDEX IF NOT EXISTS idx_provider_chats_session ON provider_chats(session_id, provider_plugin_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_items_scope ON knowledge_items(scope, project_id, kind);
CREATE INDEX IF NOT EXISTS idx_knowledge_items_source_event ON knowledge_items(source_event_id);
