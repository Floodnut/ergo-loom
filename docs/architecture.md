# Ergo Loom 아키텍처

Ergo Loom은 로컬 우선 AI 작업 컨텍스트 매니저다. 여러 AI 프로바이더를 단일 로컬 컨텍스트 그래프 위에서 운영하며, 세션·프로젝트·라우트·정책을 플러그인 방식으로 조합한다.

---

## 큰 그림

```
┌─────────────────────────────────────────────────────────────┐
│                     Browser / Desktop UI                    │
│         TypeScript · static embed · SSE + REST              │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP / SSE (localhost:3763)
┌───────────────────────────▼─────────────────────────────────┐
│                    internal/web · Server                    │
│   REST handler · run orchestration · queue · steering       │
│   session cancel registry · route exclusion cache           │
└──────┬──────────┬──────────┬──────────┬─────────────────────┘
       │          │          │          │
  ContextPacket  Route   HandoffPolicy  ToolRuntime
  Policy         Policy
       │          │          │          │
┌──────▼──────────▼──────────▼──────────▼─────────────────────┐
│              Plugin Registries (모두 교체 가능)              │
│  packetpolicy · routepolicy · handoffpolicy · toolruntime   │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                  internal/core · Domain Types               │
│  Event · Head · GraphBranch · ChatRun · ProviderSegment     │
│  ContextPacket · KnowledgeItem · QueueItem · CandidateOutput│
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│               internal/storage/sqlitecli · Store            │
│  SQLite 단일 파일 (~/.ergo-loom/local.db)                   │
│  Project · Session · Route · Message · Event · Head · Queue  │
└─────────────────────────────────────────────────────────────┘
```

---

## 레이어별 역할

### UI (`apps/desktop-or-web`)

- TypeScript 단일 파일 앱 (`src/app.ts`). Go 바이너리에 정적으로 embed.
- REST + SSE로 서버와 통신. WebSocket 없음.
- 상태: `state.project`, `state.session`, `state.plugins` 등 in-memory.
- 작곡가(composer) → 큐 → Stop 버튼 → 프로젝트 정책 UI 포함.

### Web Server (`internal/web`)

런타임의 진입점. 다음 역할을 모두 담당한다.

| 역할 | 상세 |
|---|---|
| REST API | 세션·프로젝트·라우트·플러그인 CRUD |
| Run 오케스트레이션 | `executeMainRun` → `prepareMainRun` → provider.Driver |
| 큐 관리 | `ConsumeNextQueueItem` (atomic SQL) + `maybeConsumeQueue` |
| Steering 인터럽트 | `sessionCancels` map + `context.CancelFunc` |
| 라우트 제외 캐시 | `sessionExcluded` map, 토큰 고갈 시 세션 내 영구 제외 |
| Moderation | `moderatedSelectionForActiveChat` — 라우트 만료 감지 + 핸드오프 |
| Budget 경고 | `maybeWarnBudget` (80% 도달 시 `OnBudgetWarning`) |

### Plugin Registries

Ergo Loom의 핵심 확장 지점. 모두 같은 패턴 — `Register / Get / List / GetOrDefault`.

| 패키지 | 인터페이스 | 역할 |
|---|---|---|
| `packetpolicy` | `ContextPacketPolicy` | 컨텍스트 패킷 조립 전략 |
| `routepolicy` | `RouteSelectionPolicy` | 라우트·모델 선택 전략 |
| `handoffpolicy` | `HandoffPolicy` | 프로바이더 전환 감지·요약 |
| `toolruntime` | — | 툴 승인·실행 브로커 |

등록된 정책 목록은 `GET /api/plugins`로 FE에 노출된다.

### Core Domain (`internal/core`)

순수 타입 패키지. 외부 의존 없음.

**컨텍스트 그래프 핵심 타입:**

```
Event           — 그래프의 노드. type + PayloadRef
Head            — 세션+브랜치별 현재 최신 이벤트 포인터
GraphBranch     — 브랜치 메타데이터 (fromEventID → headEventID)
ChatRun         — 단일 AI 응답 실행 단위 (main | parallel)
ProviderSegment — ChatRun 안에서 특정 프로바이더가 담당한 구간
ContextPacket   — 프로바이더에 전달되는 조립된 컨텍스트
QueueItem       — 실행 대기 메시지 (normal | steering | parallel)
CandidateOutput — parallel run의 결과물, 수락/거절 가능
KnowledgeItem   — 프로젝트·글로벌 KB 항목
```

**이벤트 타입 목록 (EventType):**

`message.user`, `message.assistant`, `provider.run.started`, `provider.run.completed`, `tool.requested`, `tool.approved`, `tool.rejected`, `tool.completed`, `tool.failed`, `turn.aborted`, `file.referenced`, `summary.created`, `branch.created`, `merge.created`, `knowledge.promoted`, `moderator.handoff`, `queue.item.created`, `queue.item.reordered`, `steering.added`, `parallel.run.queued`, `candidate.merged`

### Storage (`internal/storage/sqlitecli`)

SQLite 단일 파일 (`~/.ergo-loom/local.db`). 스키마 마이그레이션은 `Store.Init()` 에서 `ensureXxx` 함수로 처리한다.

주요 테이블: `projects`, `sessions`, `access_routes`, `provider_profiles`, `provider_models`, `messages`, `context_events`, `context_heads`, `context_branches`, `chat_runs`, `provider_segments`, `context_packets`, `chat_queue_items`, `candidate_outputs`, `knowledge_items`

---

## 메시지 전송 흐름

```
POST /api/sessions/{id}/messages
         │
         ▼
  활성 ChatRun 존재?
    Yes → QueueItem 생성 (mode: normal | steering | parallel)
    No  → executeMainRun 직접 실행
         │
         ▼
  prepareMainRun
    - context_events에서 조상 이벤트 로드
    - ContextPacketPolicy.Build() → ContextPacket 조립
    - HandoffPolicy.DetectSwitch() → 프로바이더 전환 여부 판단
         │
         ▼
  provider.Driver.Run(ctx, ContextPacket)
    - SSE로 청크 스트리밍
    - 토큰 usage 추적
         │
         ▼
  completeMainRun
    - message.assistant 이벤트 append
    - Head 이동
    - maybeConsumeQueue() → 다음 큐 아이템 실행
```

---

## 큐 동시성 모델

```
chat_queue_items
  ┌──────────┬────────────┬────────┐
  │ normal   │ steering   │parallel│
  └────┬─────┴─────┬──────┴───┬────┘
       │           │          │
       ▼           ▼          ▼
  ConsumeNextQueueItem (atomic SQL UPDATE ... NOT EXISTS)
       │
  maybeConsumeQueue
       │
  ┌────┴──────────────┐
  normal/steering      parallel
       │                   │
  cancelActiveRun()    startParallelRunFromQueueItem()
  executeMainRun()     (별도 goroutine)
```

- `ConsumeNextQueueItem`: 활성 ChatRun이 없을 때만 원자적으로 큐 소비.
- Steering: 현재 실행 중인 run을 `context.CancelFunc`로 즉시 취소 후 재실행.
- Parallel: 현재 run과 병행 실행 → `CandidateOutput`으로 결과 저장.

---

## 라우트 배제 (Token Exhaustion)

```
provider.Driver.Run() → 토큰 고갈 에러
    │
    ▼
executeMainRun → addSessionExcludedRoute(sessionID, routeID)
    │
    ▼
resolveChatSelectionWithExclusions(sessionID, ...)
    → sessionExcluded[sessionID]를 excludedRouteIDs에 합산
    → 다음 가용 라우트로 폴백
```

세션 내 한 번 소진된 라우트는 서버 재시작 전까지 해당 세션에서 제외된다.

---

## 프로젝트 정책 (Plugin Policy)

`GET /api/plugins`가 반환하는 등록된 정책 목록:

| 카테고리 | 형태 | 설명 |
|---|---|---|
| `contextPackets` | `string[]` | ContextPacket 조립 전략 |
| `handoffs` | `string[]` | 핸드오프 요약 전략 |
| `routeSelection` | `string[]` | 라우트 선택 전략 |
| `toolApproval` | `{name, label}[]` | 툴 승인 정책 |
| `kbScope` | `{name, label}[]` | KB 검색 범위 정책 |

프로젝트별 정책은 `PATCH /api/projects/{id}/settings`로 변경. 상세: `docs/plugin-policy-contract.md`, `docs/project-settings.md`.

---

## 레포지터리 구조

```
ergo-loom/
  apps/
    desktop-or-web/
      src/app.ts          # FE 단일 파일
      static/             # HTML · CSS · assets
      webapp.go           # Go embed
  internal/
    core/
      model.go            # Session · Message · Branch · Route 타입
      context_graph.go    # Event · Head · ChatRun · ContextPacket 등
      moderator.go        # Moderator 인터페이스
    web/
      server.go           # REST API + 런타임 오케스트레이션
    storage/
      sqlitecli/store.go  # SQLite 구현
    provider/
      driver.go           # DriverRegistry + Driver 인터페이스
      codex.go            # Codex provider 구현
      event.go            # provider 이벤트 타입
    packetpolicy/
      policy.go           # FlatTrimPolicy · SegmentChainPolicy
    routepolicy/
      policy.go           # FirstAvailablePolicy 등
    handoffpolicy/
      policy.go           # NopHandoffPolicy 등
    chatfilter/
      filter.go           # 채팅 필터 체인
    toolruntime/
      registry.go         # 툴 등록
      shell.go            # 쉘 툴
      event.go            # 툴 이벤트
    knowledge/
      retriever.go        # KB 검색
  docs/
    architecture.md       # 이 문서
    context-graph.md      # 컨텍스트 그래프 상세
    plugin-policy-contract.md  # 플러그인·정책 계약
    project-settings.md   # 프로젝트 설정 API
    realtime-protocol.md  # SSE 프로토콜
    desktop-app.md
    visual-direction.md
```

---

## 개발 시작

```bash
# 서버 실행 (포트 3763)
ergo app

# 빌드
go build ./...

# 타입 체크 (FE)
cd apps/desktop-or-web && npx tsc --noEmit
```

로컬 DB: `~/.ergo-loom/local.db`  
서버 주소: `http://localhost:3763`
