import { RichParameter } from "./provisionerGenerated"

// Rich parameters

const emptyParameter: RichParameter = {
  name: "",
  description: "",
  type: "",
  mutable: false,
  defaultValue: "",
  icon: "",
  options: [],
  validationRegex: "",
  validationError: "",
  validationMin: undefined,
  validationMax: undefined,
  validationMonotonic: "",
  required: false,
  displayName: "",
  order: 0,
  ephemeral: false,
}

export const firstParameter: RichParameter = {
  ...emptyParameter,

  name: "first_parameter",
  displayName: "First parameter",
  type: "number",
  options: [],
  description: "This is first parameter.",
  icon: "/emojis/1f310.png",
  defaultValue: "123",
  mutable: true,
  order: 1,
}

export const secondParameter: RichParameter = {
  ...emptyParameter,

  name: "second_parameter",
  displayName: "Second parameter",
  type: "string",
  options: [],
  description: "This is second parameter.",
  defaultValue: "abc",
  icon: "",
  mutable: false,
  required: false,
  order: 2,
}

export const thirdParameter: RichParameter = {
  ...emptyParameter,

  name: "third_parameter",
  type: "string",
  options: [],
  description: "This is third parameter.",
  mutable: false,
  required: true,
  order: 3,
}

export const fourthParameter: RichParameter = {
  ...emptyParameter,

  name: "fourth_parameter",
  type: "bool",
  options: [],
  description: "This is fourth parameter.",
  defaultValue: "true",
  icon: "",
  mutable: false,
  required: true,
  order: 3,
}

// Build options

export const firstBuildOption: RichParameter = {
  ...emptyParameter,

  name: "first_build_option",
  displayName: "First build option",
  type: "bool",
  options: [],
  description: "This is first build option.",
  defaultValue: "false",
  mutable: true,
  ephemeral: true,
  required: false,
  order: 1,
}

// TODO Options

// TODO list(string)
