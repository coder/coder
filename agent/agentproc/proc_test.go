package agentproc_test

type mockSyscaller struct {
	SetPriorityFn func(int32, int) error
}

func (f mockSyscaller) SetPriority(pid int32, nice int) error {
	if f.SetPriorityFn == nil {
		return nil
	}
	return f.SetPriorityFn(pid, nice)
}
