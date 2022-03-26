package authztest

type lookupTable struct {
	Permissions []Permission
}

func NewLookupTable(list []Permission) *lookupTable {
	return &lookupTable{
		Permissions: list,
	}
}

func (l lookupTable) Permission(idx int) Permission {
	return l.Permissions[idx]
}
