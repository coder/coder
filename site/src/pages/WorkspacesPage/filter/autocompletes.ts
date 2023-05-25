import { useMemo, useState } from "react"
import {
  BaseOption,
  OwnerOption,
  StatusOption,
  TemplateOption,
} from "./options"
import { useQuery } from "@tanstack/react-query"
import { getTemplates, getUsers } from "api/api"
import { WorkspaceStatuses } from "api/typesGenerated"
import { getDisplayWorkspaceStatus } from "utils/workspace"

type UseAutocompleteOptions<TOption extends BaseOption> = {
  id: string
  initialQuery?: string
  // Using null because of react-query
  // https://tanstack.com/query/v4/docs/react/guides/migrating-to-react-query-4#undefined-is-an-illegal-cache-value-for-successful-queries
  getInitialOption: () => Promise<TOption | null>
  getOptions: (query: string) => Promise<TOption[]>
  onChange: (option: TOption | undefined) => void
  enabled?: boolean
}

const useAutocomplete = <TOption extends BaseOption = BaseOption>({
  id,
  getInitialOption,
  getOptions,
  onChange,
  enabled,
}: UseAutocompleteOptions<TOption>) => {
  const [query, setQuery] = useState("")
  const [selectedOption, setSelectedOption] = useState<TOption>()
  const initialOptionQuery = useQuery({
    queryKey: [id, "autocomplete", "initial"],
    queryFn: () => getInitialOption(),
    onSuccess: (option) => setSelectedOption(option ?? undefined),
    enabled,
  })
  const searchOptionsQuery = useQuery({
    queryKey: [id, "autoComplete", "search"],
    queryFn: () => getOptions(query),
    enabled,
  })
  const searchOptions = useMemo(() => {
    const isDataLoaded =
      searchOptionsQuery.isFetched && initialOptionQuery.isFetched

    if (!isDataLoaded) {
      return undefined
    }

    let options = searchOptionsQuery.data as TOption[]

    if (!selectedOption) {
      return options
    }

    // We will add the initial option on the top of the options
    // 1 - remove the initial option from the search options if it exists
    // 2 - add the initial option on the top
    options = options.filter((option) => option.value !== selectedOption.value)
    options.unshift(selectedOption)

    // Filter data based o search query
    options = options.filter(
      (option) =>
        option.label.toLowerCase().includes(query.toLowerCase()) ||
        option.value.toLowerCase().includes(query.toLowerCase()),
    )

    return options
  }, [
    initialOptionQuery.isFetched,
    query,
    searchOptionsQuery.data,
    searchOptionsQuery.isFetched,
    selectedOption,
  ])

  const selectOption = (option: TOption) => {
    let newSelectedOptionValue: TOption | undefined = option

    if (option.value === selectedOption?.value) {
      newSelectedOptionValue = undefined
    }

    if (onChange) {
      onChange(newSelectedOptionValue)
    }
    setSelectedOption(newSelectedOptionValue)
  }
  const clearSelection = () => {
    setSelectedOption(undefined)
  }

  return {
    query,
    setQuery,
    selectedOption,
    selectOption,
    clearSelection,
    isInitializing: initialOptionQuery.isInitialLoading,
    initialOption: initialOptionQuery.data,
    isSearching: searchOptionsQuery.isFetching,
    searchOptions,
  }
}

export const useUsersAutocomplete = (
  initialOptionValue: string | undefined,
  onChange: (option: OwnerOption | undefined) => void,
  enabled?: boolean,
) =>
  useAutocomplete({
    id: "owner",
    getInitialOption: async () => {
      const usersRes = await getUsers({ q: initialOptionValue, limit: 1 })
      const firstUser = usersRes.users.at(0)
      if (firstUser && firstUser.username === initialOptionValue) {
        return {
          label: firstUser.username,
          value: firstUser.username,
          avatarUrl: firstUser.avatar_url,
        }
      }
      return null
    },
    getOptions: async (query) => {
      const usersRes = await getUsers({ q: query, limit: 25 })
      return usersRes.users.map((user) => ({
        label: user.username,
        value: user.username,
        avatarUrl: user.avatar_url,
      }))
    },
    onChange,
    enabled,
  })

export type UsersAutocomplete = ReturnType<typeof useUsersAutocomplete>

export const useTemplatesAutocomplete = (
  orgId: string,
  initialOptionValue: string | undefined,
  onChange: (option: TemplateOption | undefined) => void,
) => {
  return useAutocomplete({
    id: "template",
    getInitialOption: async () => {
      const templates = await getTemplates(orgId)
      const template = templates.find(
        (template) => template.name === initialOptionValue,
      )
      if (template) {
        return {
          label:
            template.display_name !== ""
              ? template.display_name
              : template.name,
          value: template.name,
          icon: template.icon,
        }
      }
      return null
    },
    getOptions: async (query) => {
      const templates = await getTemplates(orgId)
      const filteredTemplates = templates.filter(
        (template) =>
          template.name.toLowerCase().includes(query.toLowerCase()) ||
          template.display_name.toLowerCase().includes(query.toLowerCase()),
      )
      return filteredTemplates.map((template) => ({
        label:
          template.display_name !== "" ? template.display_name : template.name,
        value: template.name,
        icon: template.icon,
      }))
    },
    onChange,
  })
}

export type TemplatesAutocomplete = ReturnType<typeof useTemplatesAutocomplete>

export const useStatusAutocomplete = (
  initialOptionValue: string | undefined,
  onChange: (option: StatusOption | undefined) => void,
) => {
  const statusOptions = WorkspaceStatuses.map((status) => {
    const display = getDisplayWorkspaceStatus(status)
    return {
      label: display.text,
      value: status,
      color: display.type ?? "warning",
    } as StatusOption
  })
  return useAutocomplete({
    id: "status",
    getInitialOption: async () =>
      statusOptions.find((option) => option.value === initialOptionValue) ??
      null,
    getOptions: async () => statusOptions,
    onChange,
  })
}

export type StatusAutocomplete = ReturnType<typeof useStatusAutocomplete>
