import { BaseOption } from "components/Filter/options"

export type UserOption = BaseOption & {
  avatarUrl?: string
}

export type StatusOption = BaseOption & {
  color: string
}

export type TemplateOption = BaseOption & {
  icon?: string
}
