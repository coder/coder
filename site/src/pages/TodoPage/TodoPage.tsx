import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { cn } from "utils/cn";
import { type FC, type FormEvent, useMemo, useState } from "react";
import { Trash2 } from "lucide-react";

type TodoItem = {
	id: string;
	title: string;
	completed: boolean;
	createdAt: number;
};

const buildTodo = (title: string): TodoItem => {
	return {
		id:
			typeof crypto !== "undefined" && "randomUUID" in crypto
				? crypto.randomUUID()
				: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
		title,
		completed: false,
		createdAt: Date.now(),
	};
};

const TodoPage: FC = () => {
	const [todos, setTodos] = useState<TodoItem[]>([]);
	const [pendingTitle, setPendingTitle] = useState("");

	const remainingCount = useMemo(
		() => todos.filter((todo) => !todo.completed).length,
		[todos],
	);
	const completedCount = todos.length - remainingCount;

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		const trimmedTitle = pendingTitle.trim();

		if (trimmedTitle.length === 0) {
			return;
		}

		setTodos((current) => [buildTodo(trimmedTitle), ...current]);
		setPendingTitle("");
	};

	const handleToggle = (id: string, checked: boolean) => {
		setTodos((current) =>
			current.map((todo) =>
				todo.id === id
					? {
							...todo,
							completed: checked,
						}
					: todo,
			),
		);
	};

	const handleRemove = (id: string) => {
		setTodos((current) => current.filter((todo) => todo.id !== id));
	};

	const clearCompleted = () => {
		setTodos((current) => current.filter((todo) => !todo.completed));
	};

	const hasTodos = todos.length > 0;
	const hasCompleted = completedCount > 0;

	return (
		<main className="flex min-h-screen items-center justify-center bg-surface-primary px-4 py-12">
			<title>Shadcn To-Do List</title>
			<section className="w-full max-w-2xl space-y-8 rounded-2xl border border-border bg-surface-secondary/60 p-8 shadow-lg">
				<header className="space-y-3 text-center">
					<p className="text-xs font-semibold uppercase tracking-wide text-content-secondary">
						Demo
					</p>
					<h1 className="text-3xl font-semibold text-content-primary">
						To-do list
					</h1>
					<p className="text-sm text-content-secondary">
						Organize your day with a lightweight task list built with shadcn/ui
						components.
					</p>
				</header>

				<form className="space-y-3" onSubmit={handleSubmit}>
					<div className="flex flex-col gap-3 sm:flex-row">
						<Input
							aria-label="Task description"
							placeholder="What do you want to get done?"
							value={pendingTitle}
							onChange={(event) => setPendingTitle(event.target.value)}
							autoFocus
						/>
						<Button
							className="sm:self-start"
							type="submit"
							disabled={pendingTitle.trim().length === 0}
						>
							Add task
						</Button>
					</div>
					<div className="flex flex-wrap items-center justify-between gap-2 text-sm text-content-secondary">
						<span>
							{remainingCount === 0
								? "You're all caught up!"
								: `${remainingCount} task${remainingCount === 1 ? "" : "s"} remaining`}
						</span>
						{hasCompleted && (
							<Button
								type="button"
								variant="subtle"
								size="sm"
								onClick={clearCompleted}
								className="px-3"
							>
								Clear completed ({completedCount})
							</Button>
						)}
					</div>
				</form>

				<div className="space-y-3">
					{!hasTodos && (
						<div className="rounded-xl border border-dashed border-border bg-surface-primary/60 px-6 py-12 text-center">
							<p className="text-sm font-medium text-content-secondary">
								No tasks yet
							</p>
							<p className="text-xs text-content-secondary">
								Add your first task to see it appear in the list.
							</p>
						</div>
					)}

					{hasTodos && (
						<ul className="space-y-3">
							{todos.map((todo) => {
								const checkboxId = `todo-${todo.id}`;
								return (
									<li
										key={todo.id}
										className="flex items-center justify-between gap-4 rounded-xl border border-border bg-surface-primary px-4 py-3 shadow-sm"
									>
										<div className="flex grow items-center gap-3">
											<Checkbox
												id={checkboxId}
												checked={todo.completed}
												onCheckedChange={(value) =>
													handleToggle(todo.id, value === true)
												}
												aria-labelledby={`${checkboxId}-label`}
											/>
											<Label
												id={`${checkboxId}-label`}
												htmlFor={checkboxId}
												className={cn(
													"text-base text-content-primary",
													todo.completed &&
														"text-content-secondary line-through",
												)}
											>
												{todo.title}
											</Label>
										</div>
										<Button
											variant="subtle"
											size="icon"
											type="button"
											onClick={() => handleRemove(todo.id)}
											aria-label={`Delete ${todo.title}`}
										>
											<Trash2 className="h-4 w-4" aria-hidden />
										</Button>
									</li>
								);
							})}
						</ul>
					)}
				</div>
			</section>
		</main>
	);
};

export default TodoPage;
