export type BaseOption = {
  label: string
  value: string
}

export type OwnerOption = BaseOption & {
  avatarUrl?: string
}

export type StatusOption = BaseOption & {
  color: string
}

export type TemplateOption = BaseOption & {
  icon?: string
}
