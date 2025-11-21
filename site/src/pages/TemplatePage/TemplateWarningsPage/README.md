# Template Warnings Page

This page displays warnings and errors for the active template version, providing tips and suggestions on how to better format Terraform code.

## Features

- **Table-based list view** with organized columns for easy scanning
- **Minimize/Restore warnings** - click the X button to minimize warnings (moves to bottom) or ↻ to restore
- **Smart sorting** - Active warnings at top, dismissed warnings at bottom
- **Three severity levels**: Error, Warning, and Info with color-coded pills and icons
- **Empty state** when no warnings exist
- **Optional error codes** displayed in monospace badges
- **Visual feedback** - Dismissed warnings show reduced opacity and simplified view
- **Clean, professional design** following Coder's design system patterns

## Backend Integration

The page currently uses a placeholder `useTemplateWarnings` hook that returns an empty array. You'll need to:

1. **Create an API endpoint** in the Go backend that returns warnings for a template version
2. **Add the API method** to `site/src/api/api.ts` (e.g., `getTemplateVersionWarnings`)
3. **Replace the hook** with an actual React Query hook that fetches data

### Expected Warning Structure

```typescript
interface Warning {
  id: string; // Unique identifier for dismissal tracking
  severity: "error" | "warning" | "info";
  title: string;
  message: string;
  code?: string; // Optional error code (e.g., "TF001")
  dismissed?: boolean; // Optional - marks warning as dismissed from backend
}
```

**Note:** The component tracks dismissed state both from:
1. Backend-provided `dismissed` attribute (persisted across sessions)
2. Client-side state (temporary, resets on page reload)

### Example Implementation

```typescript
// In site/src/api/api.ts
export const getTemplateVersionWarnings = async (
  versionId: string,
): Promise<Warning[]> => {
  const response = await axios.get(
    `/api/v2/templateversions/${versionId}/warnings`,
  );
  return response.data;
};

// In TemplateWarningsPage.tsx - replace the placeholder hook
const useTemplateWarnings = (templateVersionId: string): Warning[] => {
  const { data } = useQuery({
    queryKey: ['templateWarnings', templateVersionId],
    queryFn: () => API.getTemplateVersionWarnings(templateVersionId),
  });
  return data ?? [];
};
```

## UI Components

The page uses Coder's standard table components:
- `Table`, `TableHeader`, `TableBody`, `TableRow`, `TableCell`, `TableHead` from `components/Table/Table`
- `Pill` component for severity badges
- `EmptyState` component for empty states
- Icons from `lucide-react` (CircleAlertIcon, TriangleAlertIcon, InfoIcon, XIcon, RotateCcwIcon)
- Theme-aware colors and consistent typography

### Table Columns

1. **Icon** (40px) - Visual severity indicator (dimmed when dismissed)
2. **Severity** (120px) - Colored pill with severity level (hidden when dismissed)
3. **Issue** (flexible) - Warning title and detailed message (simplified when dismissed)
4. **Code** (100px) - Optional error code in monospace (hidden when dismissed)
5. **Actions** (40px) - Toggle button (X to dismiss, ↻ to restore)

### Dismissed State

When a warning is dismissed:
- **Opacity reduced** to 50%
- **Moved to bottom** of the list
- **Severity pill hidden**
- **Message hidden** (only title shown in italic)
- **Code hidden**
- **Icon dimmed**
- **Button changes** to restore icon (↻)

## Navigation

The Warnings tab is visible to all users and appears after Insights in the template tabs:
- Docs
- Source Code (if user can update template)
- Resources
- Versions
- Embed
- Insights (if user has permissions)
- **Warnings** (always visible)

## Future Enhancements

Potential improvements for the backend implementation:

1. **Persist dismissed state**: Save dismissed warnings to backend so they persist across sessions
2. **File locations**: Add file name and line number to each warning
3. **Quick fixes**: Provide automated fix suggestions
4. **Filtering**: Allow filtering by severity level (show/hide dismissed)
5. **Categorization**: Group warnings by type (security, best practices, etc.)
6. **Historical data**: Show warning trends across versions
7. **Bulk actions**: Dismiss all warnings of a certain type
