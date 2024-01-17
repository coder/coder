/**
 * @file Defines a controlled searchbox component for processing form state.
 *
 * Not defined as a top-level component just yet, because it's not clear how
 * reusable this is outside of workspace dropdowns.
 */
import {
  type ForwardedRef,
  type KeyboardEvent,
  type InputHTMLAttributes,
  forwardRef,
  useId,
} from "react";
import { Search, SearchInput } from "components/Menu/Search";

interface SearchBoxProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  value: string;
  onKeyDown?: (event: KeyboardEvent) => void;
  onValueChange: (newValue: string) => void;
}

export const SearchBox = forwardRef(function SearchBox(
  props: SearchBoxProps,
  ref?: ForwardedRef<HTMLInputElement>,
) {
  const {
    onValueChange,
    onKeyDown,
    label = "Search",
    placeholder = "Search...",
    ...attrs
  } = props;

  const hookId = useId();
  const inputId = `${hookId}-${SearchBox.name}-input`;

  return (
    <Search>
      <SearchInput
        label={label}
        ref={ref}
        id={inputId}
        autoFocus
        tabIndex={0}
        placeholder={placeholder}
        {...attrs}
        onKeyDown={onKeyDown}
        onChange={(e) => onValueChange(e.target.value)}
      />
    </Search>
  );
});
