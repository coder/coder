package chatd

//go:generate go tool mockgen -source=tasks.go -destination ./mock_task_side_effects_internal_test.go -package chatd -mock_names taskSideEffects=MockTaskSideEffects taskSideEffects
