package sqlitecli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Floodnut/ergo-loom/internal/core"
)

func TestAddMessageRecordsContextEvents(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	session, err := store.CreateChatSessionForProject("default", "Graph test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := store.AddMessage(session.ID, "user", "hello"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	if _, err := store.AddMessage(session.ID, "assistant", "hi"); err != nil {
		t.Fatalf("add assistant message: %v", err)
	}

	head, err := store.getHead(session.ProjectID, session.ID, "main")
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	events, err := store.ListAncestors(head.EventID, 10)
	if err != nil {
		t.Fatalf("list ancestors: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != core.EventMessageUser {
		t.Fatalf("expected first event to be user message, got %s", events[0].Type)
	}
	if events[1].Type != core.EventMessageAssistant {
		t.Fatalf("expected second event to be assistant message, got %s", events[1].Type)
	}
	if len(events[1].ParentEventIDs) != 1 || events[1].ParentEventIDs[0] != events[0].ID {
		t.Fatalf("expected assistant event to parent user event, got %#v", events[1].ParentEventIDs)
	}
	for _, event := range events {
		path := filepath.Join(dir, "objects", "events", event.ID+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected context event object %s: %v", path, err)
		}
	}
}

func TestKnowledgeItemsAreSearchableAndFileBacked(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	item, err := store.AddKnowledgeItem(core.KnowledgeItem{
		Scope:      core.KnowledgeScopeGlobal,
		Kind:       "procedure",
		Title:      "Claude subscription auth uses setup-token",
		ContentRef: "",
	})
	if err != nil {
		t.Fatalf("add knowledge item: %v", err)
	}
	if item.ContentRef == "" {
		t.Fatal("expected content ref")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(item.ContentRef))); err != nil {
		t.Fatalf("expected knowledge object: %v", err)
	}

	items, err := store.SearchKnowledge(KnowledgeSearchOptions{
		Query: "subscription",
		Scope: core.KnowledgeScopeGlobal,
	})
	if err != nil {
		t.Fatalf("search knowledge: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("expected knowledge item %s, got %#v", item.ID, items)
	}
}

func TestChatRunTracksProviderSegments(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	session, err := store.CreateChatSessionForProject("default", "Run test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := store.AddMessage(session.ID, "user", "run this"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	head, err := store.GetHead(session.ProjectID, session.ID, "main")
	if err != nil {
		t.Fatalf("get head: %v", err)
	}

	run, err := store.StartChatRun(ChatRunInput{
		ProjectID:       session.ProjectID,
		SessionID:       session.ID,
		BranchID:        "main",
		Role:            core.ChatRunRoleMain,
		Status:          core.ChatRunRunning,
		InputEventID:    head.EventID,
		ContextPacketID: "packet_1",
	})
	if err != nil {
		t.Fatalf("start chat run: %v", err)
	}
	segment, err := store.StartProviderSegment(ProviderSegmentInput{
		ChatRunID:  run.ID,
		ProviderID: "anthropic",
		RouteID:    "claude-code-cli",
		ModelID:    "anthropic-claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("start provider segment: %v", err)
	}
	active, err := store.ActiveMainChatRun(session.ID)
	if err != nil {
		t.Fatalf("active main chat run: %v", err)
	}
	if active.ID != run.ID {
		t.Fatalf("expected active run %s, got %s", run.ID, active.ID)
	}
	runningSegment, err := store.ActiveProviderSegment(run.ID)
	if err != nil {
		t.Fatalf("active provider segment: %v", err)
	}
	if runningSegment.ID != segment.ID {
		t.Fatalf("expected active segment %s, got %s", segment.ID, runningSegment.ID)
	}
	steering, err := store.RecordSteering(SteeringInput{
		ChatRunID:         run.ID,
		ProviderSegmentID: segment.ID,
		Content:           "avoid electron changes",
	})
	if err != nil {
		t.Fatalf("record steering: %v", err)
	}
	if steering.ChatRunID != run.ID || steering.ProviderSegmentID != segment.ID {
		t.Fatalf("unexpected steering record: %#v", steering)
	}
	segment, err = store.CompleteProviderSegment(segment.ID, core.ProviderSegmentCompleted, "thread_1")
	if err != nil {
		t.Fatalf("complete provider segment: %v", err)
	}
	if segment.Status != core.ProviderSegmentCompleted || segment.ExternalThreadID != "thread_1" {
		t.Fatalf("unexpected provider segment: %#v", segment)
	}
	run, err = store.CompleteChatRun(run.ID, core.ChatRunCompleted, head.EventID)
	if err != nil {
		t.Fatalf("complete chat run: %v", err)
	}
	if run.Status != core.ChatRunCompleted || run.OutputEventID != head.EventID {
		t.Fatalf("unexpected chat run: %#v", run)
	}
}

func TestContextPacketIsPersistedAsProjectionAndObject(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	session, err := store.CreateChatSessionForProject("default", "Packet test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	packet := core.ContextPacket{
		ID:        "context_packet_test",
		ProjectID: session.ProjectID,
		SessionID: session.ID,
		BranchID:  "main",
		UserInput: "hello",
		Content:   "context\nhello",
		References: []core.ContextReference{
			{Kind: "message.user", ID: "event_1", Ref: "message:1"},
		},
	}
	record, err := store.SaveContextPacket(packet)
	if err != nil {
		t.Fatalf("save context packet: %v", err)
	}
	if record.ReferenceCount != 1 {
		t.Fatalf("expected one reference, got %d", record.ReferenceCount)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(record.ContentRef))); err != nil {
		t.Fatalf("expected context packet object: %v", err)
	}
}

func TestQueueItemsCanBePersistedReorderedAndConsumed(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	session, err := store.CreateChatSessionForProject("default", "Queue test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	first, err := store.AddQueueItem(QueueItemInput{
		SessionID: session.ID,
		Content:   "first",
		Mode:      core.QueueItemSteering,
	})
	if err != nil {
		t.Fatalf("add first queue item: %v", err)
	}
	second, err := store.AddQueueItem(QueueItemInput{
		SessionID: session.ID,
		Content:   "second",
		Mode:      core.QueueItemParallel,
	})
	if err != nil {
		t.Fatalf("add second queue item: %v", err)
	}
	if err := store.ReorderQueueItems(session.ID, []string{second.ID, first.ID}); err != nil {
		t.Fatalf("reorder queue: %v", err)
	}
	items, err := store.ListPendingQueueItems(session.ID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("unexpected queue order: %#v", items)
	}
	if _, err := store.UpdateQueueItemStatus(second.ID, core.QueueItemConsumed); err != nil {
		t.Fatalf("consume queue item: %v", err)
	}
	items, err = store.ListPendingQueueItems(session.ID)
	if err != nil {
		t.Fatalf("list queue after consume: %v", err)
	}
	if len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("unexpected remaining queue: %#v", items)
	}
}

func TestCandidateOutputsAreSeparateFromMainTranscript(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	session, err := store.CreateChatSessionForProject("default", "Candidate test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	run, err := store.StartChatRun(ChatRunInput{
		ProjectID: session.ProjectID,
		SessionID: session.ID,
		BranchID:  "main",
		Role:      core.ChatRunRoleParallel,
		Status:    core.ChatRunCompleted,
	})
	if err != nil {
		t.Fatalf("start parallel run: %v", err)
	}
	candidate, err := store.AddCandidateOutput(CandidateOutputInput{
		ChatRunID: run.ID,
		SessionID: session.ID,
		BranchID:  "main",
		Content:   "candidate reply",
		Status:    core.CandidateOutputReady,
	})
	if err != nil {
		t.Fatalf("add candidate output: %v", err)
	}
	if candidate.Status != core.CandidateOutputReady {
		t.Fatalf("unexpected candidate status: %#v", candidate)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(candidate.ContentRef))); err != nil {
		t.Fatalf("expected candidate object: %v", err)
	}
	accepted, err := store.UpdateCandidateOutputStatus(candidate.ID, core.CandidateOutputAccepted)
	if err != nil {
		t.Fatalf("accept candidate: %v", err)
	}
	if accepted.Status != core.CandidateOutputAccepted {
		t.Fatalf("unexpected accepted candidate: %#v", accepted)
	}
}

func TestMergeCandidateOutputMaterializesAssistantMessage(t *testing.T) {
	dir := t.TempDir()
	store := Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: "schema.sql",
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	session, err := store.CreateChatSessionForProject("default", "Candidate merge")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := store.AddMessage(session.ID, "user", "compare two answers"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	head, err := store.GetHead(session.ProjectID, session.ID, "main")
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	triggerEventID := head.EventID
	run, err := store.StartChatRun(ChatRunInput{
		ProjectID:    session.ProjectID,
		SessionID:    session.ID,
		BranchID:     "main",
		Role:         core.ChatRunRoleParallel,
		Status:       core.ChatRunCompleted,
		InputEventID: triggerEventID,
	})
	if err != nil {
		t.Fatalf("start parallel run: %v", err)
	}
	first, err := store.AddCandidateOutput(CandidateOutputInput{
		ChatRunID:      run.ID,
		SessionID:      session.ID,
		BranchID:       "main",
		TriggerEventID: triggerEventID,
		Content:        "candidate one",
		Status:         core.CandidateOutputReady,
	})
	if err != nil {
		t.Fatalf("add first candidate: %v", err)
	}
	second, err := store.AddCandidateOutput(CandidateOutputInput{
		ChatRunID:      run.ID,
		SessionID:      session.ID,
		BranchID:       "main",
		TriggerEventID: triggerEventID,
		Content:        "candidate two",
		Status:         core.CandidateOutputReady,
	})
	if err != nil {
		t.Fatalf("add second candidate: %v", err)
	}

	result, err := store.MergeCandidateOutput(first.ID)
	if err != nil {
		t.Fatalf("merge candidate: %v", err)
	}
	if result.Candidate.Status != core.CandidateOutputMerged {
		t.Fatalf("expected merged candidate, got %#v", result.Candidate)
	}
	if result.Message.Role != "assistant" || result.Message.Content != "candidate one" {
		t.Fatalf("unexpected materialized message: %#v", result.Message)
	}
	if result.Event.Type != core.EventCandidateMerged {
		t.Fatalf("unexpected merge event: %#v", result.Event)
	}
	if len(result.SupersededCandidateIDs) != 1 || result.SupersededCandidateIDs[0] != second.ID {
		t.Fatalf("unexpected superseded ids: %#v", result.SupersededCandidateIDs)
	}
	pending, err := store.ListPendingCandidateOutputs(session.ID)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected merged/superseded candidates hidden, got %#v", pending)
	}
	_, messages, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if len(messages) != 2 || messages[1].Content != "candidate one" {
		t.Fatalf("unexpected transcript: %#v", messages)
	}
}
