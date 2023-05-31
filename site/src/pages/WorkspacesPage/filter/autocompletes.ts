import { useMemo, useRef, useState } from "react"
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
import { useMe } from "hooks"

type UseAutocompleteOptions<TOption extends BaseOption> = {
  id: string
  value: string | undefined
  // Using null because of react-query
  // https://tanstack.com/query/v4/docs/react/guides/migrating-to-react-query-4#undefined-is-an-illegal-cache-value-for-successful-queries
  getSelectedOption: () => Promise<TOption | null>
  getOptions: (query: string) => Promise<TOption[]>
  onChange: (option: TOption | undefined) => void
  enabled?: boolean
}

const useAutocomplete = <TOption extends BaseOption = BaseOption>({
  id,
  value,
  getSelectedOption,
  getOptions,
  onChange,
  enabled,
}: UseAutocompleteOptions<TOption>) => {
  const selectedOptionsCacheRef = useRef<Record<string, TOption>>({})
  const [query, setQuery] = useState("")
  const selectedOptionQuery = useQuery({
    queryKey: [id, "autocomplete", "selected", value],
    queryFn: () => {
      if (!value) {
        return null
      }

      const cachedOption = selectedOptionsCacheRef.current[value]
      if (cachedOption) {
        return cachedOption
      }

      return getSelectedOption()
    },
    enabled,
    keepPreviousData: true,
  })
  const selectedOption = selectedOptionQuery.data
  const searchOptionsQuery = useQuery({
    queryKey: [id, "autocomplete", "search", query],
    queryFn: () => getOptions(query),
    enabled,
  })
  const searchOptions = useMemo(() => {
    const isDataLoaded =
      searchOptionsQuery.isFetched && selectedOptionQuery.isFetched

    if (!isDataLoaded) {
      return undefined
    }

    let options = searchOptionsQuery.data as TOption[]

    if (selectedOption) {
      options = options.filter(
        (option) => option.value !== selectedOption.value,
      )
      options = [selectedOption, ...options]
    }

    options = options.filter(
      (option) =>
        option.label.toLowerCase().includes(query.toLowerCase()) ||
        option.value.toLowerCase().includes(query.toLowerCase()),
    )

    return options
  }, [
    selectedOptionQuery.isFetched,
    query,
    searchOptionsQuery.data,
    searchOptionsQuery.isFetched,
    selectedOption,
  ])

  const selectOption = (option: TOption) => {
    let newSelectedOptionValue: TOption | undefined = option
    selectedOptionsCacheRef.current[option.value] = option
    setQuery("")

    if (option.value === selectedOption?.value) {
      newSelectedOptionValue = undefined
    }

    onChange(newSelectedOptionValue)
  }

  return {
    query,
    setQuery,
    selectedOption,
    selectOption,
    searchOptions,
    isInitializing: selectedOptionQuery.isInitialLoading,
    initialOption: selectedOptionQuery.data,
    isSearching: searchOptionsQuery.isFetching,
  }
}

export const useUsersAutocomplete = (
  value: string | undefined,
  onChange: (option: OwnerOption | undefined) => void,
  enabled?: boolean,
) => {
  const me = useMe()

  const addMeAsFirstOption = (options: OwnerOption[]) => {
    options = options.filter((option) => option.value !== me.username)
    return [
      { label: me.username, value: me.username, avatarUrl: me.avatar_url },
      ...options,
    ]
  }

  return useAutocomplete({
    onChange,
    enabled,
    value,
    id: "owner",
    getSelectedOption: async () => {
      const usersRes = await getUsers({ q: value, limit: 1 })
      const firstUser = usersRes.users.at(0)
      if (firstUser && firstUser.username === value) {
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
      let options: OwnerOption[] = usersRes.users.map((user) => ({
        label: user.username,
        value: user.username,
        avatarUrl: user.avatar_url,
      }))
      options = addMeAsFirstOption(options)
      return options
    },
  })
}

export type UsersAutocomplete = ReturnType<typeof useUsersAutocomplete>

export const useTemplatesAutocomplete = (
  orgId: string,
  value: string | undefined,
  onChange: (option: TemplateOption | undefined) => void,
) => {
  return useAutocomplete({
    onChange,
    value,
    id: "template",
    getSelectedOption: async () => {
      const templates = await getTemplates(orgId)
      const template = templates.find((template) => template.name === value)
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
  })
}

export type TemplatesAutocomplete = ReturnType<typeof useTemplatesAutocomplete>

export const useStatusAutocomplete = (
  value: string | undefined,
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
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      statusOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => statusOptions,
  })
}

export type StatusAutocomplete = ReturnType<typeof useStatusAutocomplete>
