# Search Provider Interface

Ergo Loom search starts with local projects and chats, but the UI is designed to accept additional providers later.

```ts
type SearchProvider = {
  id: string;
  label: string;
  search(query: string): Promise<SearchResult[]>;
};

type SearchResult = {
  id: string;
  type: "project" | "chat" | "file" | "external";
  title: string;
  detail: string;
  provider?: string;
};
```

Rules:

- Local search must search project names and chat sessions first.
- External providers should return normalized `SearchResult` objects.
- Provider-specific credentials and rate limits belong outside the modal UI.
- Selecting a result should either open the local object or hand off to the provider adapter.
