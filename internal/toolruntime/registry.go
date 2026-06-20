package toolruntime

type Definition struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	Kind             string `json:"kind"`
	Transport        string `json:"transport"`
	RequiresApproval bool   `json:"requiresApproval"`
	Enabled          bool   `json:"enabled"`
}

type Registry interface {
	List() []Definition
	Get(id string) (Definition, bool)
}

type StaticRegistry struct {
	items map[string]Definition
}

func NewStaticRegistry(definitions []Definition) StaticRegistry {
	items := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		items[definition.ID] = definition
	}
	return StaticRegistry{items: items}
}

func (r StaticRegistry) List() []Definition {
	definitions := make([]Definition, 0, len(r.items))
	for _, definition := range r.items {
		definitions = append(definitions, definition)
	}
	return definitions
}

func (r StaticRegistry) Get(id string) (Definition, bool) {
	definition, ok := r.items[id]
	return definition, ok
}
