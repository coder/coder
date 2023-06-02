import { UserOption, StatusOption, TemplateOption } from "./options"
import { getTemplates, getUsers } from "api/api"
import { WorkspaceStatuses } from "api/typesGenerated"
import { getDisplayWorkspaceStatus } from "utils/workspace"
import { useMe } from "hooks"
import { UseFilterMenuOptions, useFilterMenu } from "components/Filter/menu"

export const useUserFilterMenu = ({
  value,
  onChange,
  enabled,
}: Pick<
  UseFilterMenuOptions<UserOption>,
  "value" | "onChange" | "enabled"
>) => {
  const me = useMe()

  const addMeAsFirstOption = (options: UserOption[]) => {
    options = options.filter((option) => option.value !== me.username)
    return [
      { label: me.username, value: me.username, avatarUrl: me.avatar_url },
      ...options,
    ]
  }

  return useFilterMenu({
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
      let options: UserOption[] = usersRes.users.map((user) => ({
        label: user.username,
        value: user.username,
        avatarUrl: user.avatar_url,
      }))
      options = addMeAsFirstOption(options)
      return options
    },
  })
}

export type UserFilterMenu = ReturnType<typeof useUserFilterMenu>

export const useTemplateFilterMenu = ({
  value,
  onChange,
  orgId,
}: { orgId: string } & Pick<
  UseFilterMenuOptions<TemplateOption>,
  "value" | "onChange"
>) => {
  return useFilterMenu({
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

export type TemplateFilterMenu = ReturnType<typeof useTemplateFilterMenu>

export const useStatusFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<StatusOption>, "value" | "onChange">) => {
  const statusOptions = WorkspaceStatuses.map((status) => {
    const display = getDisplayWorkspaceStatus(status)
    return {
      label: display.text,
      value: status,
      color: display.type ?? "warning",
    } as StatusOption
  })
  return useFilterMenu({
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      statusOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => statusOptions,
  })
}

export type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>
