/* eslint-disable */
import * as _m0 from "protobufjs/minimal"
import { Observable } from "rxjs"

export const protobufPackage = "provisioner"

/** LogLevel represents severity of the log. */
export enum LogLevel {
  TRACE = 0,
  DEBUG = 1,
  INFO = 2,
  WARN = 3,
  ERROR = 4,
  UNRECOGNIZED = -1,
}

export enum AppSharingLevel {
  OWNER = 0,
  AUTHENTICATED = 1,
  PUBLIC = 2,
  UNRECOGNIZED = -1,
}

export enum WorkspaceTransition {
  START = 0,
  STOP = 1,
  DESTROY = 2,
  UNRECOGNIZED = -1,
}

/** Empty indicates a successful request/response. */
export interface Empty {}

/** TemplateVariable represents a Terraform variable. */
export interface TemplateVariable {
  name: string
  description: string
  type: string
  defaultValue: string
  required: boolean
  sensitive: boolean
}

/** RichParameterOption represents a singular option that a parameter may expose. */
export interface RichParameterOption {
  name: string
  description: string
  value: string
  icon: string
}

/** RichParameter represents a variable that is exposed. */
export interface RichParameter {
  name: string
  description: string
  type: string
  mutable: boolean
  defaultValue: string
  icon: string
  options: RichParameterOption[]
  validationRegex: string
  validationError: string
  validationMin?: number | undefined
  validationMax?: number | undefined
  validationMonotonic: string
  required: boolean
  legacyVariableName: string
  displayName: string
}

/** RichParameterValue holds the key/value mapping of a parameter. */
export interface RichParameterValue {
  name: string
  value: string
}

/** VariableValue holds the key/value mapping of a Terraform variable. */
export interface VariableValue {
  name: string
  value: string
  sensitive: boolean
}

/** Log represents output from a request. */
export interface Log {
  level: LogLevel
  output: string
}

export interface InstanceIdentityAuth {
  instanceId: string
}

export interface GitAuthProvider {
  id: string
  accessToken: string
}

/** Agent represents a running agent on the workspace. */
export interface Agent {
  id: string
  name: string
  env: { [key: string]: string }
  startupScript: string
  operatingSystem: string
  architecture: string
  directory: string
  apps: App[]
  token?: string | undefined
  instanceId?: string | undefined
  connectionTimeoutSeconds: number
  troubleshootingUrl: string
  motdFile: string
  /** Field 14 was bool login_before_ready = 14, now removed. */
  startupScriptTimeoutSeconds: number
  shutdownScript: string
  shutdownScriptTimeoutSeconds: number
  metadata: Agent_Metadata[]
  startupScriptBehavior: string
}

export interface Agent_Metadata {
  key: string
  displayName: string
  script: string
  interval: number
  timeout: number
}

export interface Agent_EnvEntry {
  key: string
  value: string
}

/** App represents a dev-accessible application on the workspace. */
export interface App {
  /**
   * slug is the unique identifier for the app, usually the name from the
   * template. It must be URL-safe and hostname-safe.
   */
  slug: string
  displayName: string
  command: string
  url: string
  icon: string
  subdomain: boolean
  healthcheck: Healthcheck | undefined
  sharingLevel: AppSharingLevel
  external: boolean
}

/** Healthcheck represents configuration for checking for app readiness. */
export interface Healthcheck {
  url: string
  interval: number
  threshold: number
}

/** Resource represents created infrastructure. */
export interface Resource {
  name: string
  type: string
  agents: Agent[]
  metadata: Resource_Metadata[]
  hide: boolean
  icon: string
  instanceType: string
  dailyCost: number
}

export interface Resource_Metadata {
  key: string
  value: string
  sensitive: boolean
  isNull: boolean
}

/** Parse consumes source-code from a directory to produce inputs. */
export interface Parse {}

export interface Parse_Request {
  directory: string
}

export interface Parse_Complete {
  templateVariables: TemplateVariable[]
}

export interface Parse_Response {
  log?: Log | undefined
  complete?: Parse_Complete | undefined
}

/**
 * Provision consumes source-code from a directory to produce resources.
 * Exactly one of Plan or Apply must be provided in a single session.
 */
export interface Provision {}

export interface Provision_Metadata {
  coderUrl: string
  workspaceTransition: WorkspaceTransition
  workspaceName: string
  workspaceOwner: string
  workspaceId: string
  workspaceOwnerId: string
  workspaceOwnerEmail: string
  templateName: string
  templateVersion: string
  workspaceOwnerOidcAccessToken: string
  workspaceOwnerSessionToken: string
}

/**
 * Config represents execution configuration shared by both Plan and
 * Apply commands.
 */
export interface Provision_Config {
  directory: string
  state: Uint8Array
  metadata: Provision_Metadata | undefined
  provisionerLogLevel: string
}

export interface Provision_Plan {
  config: Provision_Config | undefined
  richParameterValues: RichParameterValue[]
  variableValues: VariableValue[]
  gitAuthProviders: GitAuthProvider[]
}

export interface Provision_Apply {
  config: Provision_Config | undefined
  plan: Uint8Array
}

export interface Provision_Cancel {}

export interface Provision_Request {
  plan?: Provision_Plan | undefined
  apply?: Provision_Apply | undefined
  cancel?: Provision_Cancel | undefined
}

export interface Provision_Complete {
  state: Uint8Array
  error: string
  resources: Resource[]
  parameters: RichParameter[]
  gitAuthProviders: string[]
  plan: Uint8Array
}

export interface Provision_Response {
  log?: Log | undefined
  complete?: Provision_Complete | undefined
}

export const Empty = {
  encode(_: Empty, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer
  },
}

export const TemplateVariable = {
  encode(
    message: TemplateVariable,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name)
    }
    if (message.description !== "") {
      writer.uint32(18).string(message.description)
    }
    if (message.type !== "") {
      writer.uint32(26).string(message.type)
    }
    if (message.defaultValue !== "") {
      writer.uint32(34).string(message.defaultValue)
    }
    if (message.required === true) {
      writer.uint32(40).bool(message.required)
    }
    if (message.sensitive === true) {
      writer.uint32(48).bool(message.sensitive)
    }
    return writer
  },
}

export const RichParameterOption = {
  encode(
    message: RichParameterOption,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name)
    }
    if (message.description !== "") {
      writer.uint32(18).string(message.description)
    }
    if (message.value !== "") {
      writer.uint32(26).string(message.value)
    }
    if (message.icon !== "") {
      writer.uint32(34).string(message.icon)
    }
    return writer
  },
}

export const RichParameter = {
  encode(
    message: RichParameter,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name)
    }
    if (message.description !== "") {
      writer.uint32(18).string(message.description)
    }
    if (message.type !== "") {
      writer.uint32(26).string(message.type)
    }
    if (message.mutable === true) {
      writer.uint32(32).bool(message.mutable)
    }
    if (message.defaultValue !== "") {
      writer.uint32(42).string(message.defaultValue)
    }
    if (message.icon !== "") {
      writer.uint32(50).string(message.icon)
    }
    for (const v of message.options) {
      RichParameterOption.encode(v!, writer.uint32(58).fork()).ldelim()
    }
    if (message.validationRegex !== "") {
      writer.uint32(66).string(message.validationRegex)
    }
    if (message.validationError !== "") {
      writer.uint32(74).string(message.validationError)
    }
    if (message.validationMin !== undefined) {
      writer.uint32(80).int32(message.validationMin)
    }
    if (message.validationMax !== undefined) {
      writer.uint32(88).int32(message.validationMax)
    }
    if (message.validationMonotonic !== "") {
      writer.uint32(98).string(message.validationMonotonic)
    }
    if (message.required === true) {
      writer.uint32(104).bool(message.required)
    }
    if (message.legacyVariableName !== "") {
      writer.uint32(114).string(message.legacyVariableName)
    }
    if (message.displayName !== "") {
      writer.uint32(122).string(message.displayName)
    }
    return writer
  },
}

export const RichParameterValue = {
  encode(
    message: RichParameterValue,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name)
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value)
    }
    return writer
  },
}

export const VariableValue = {
  encode(
    message: VariableValue,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name)
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value)
    }
    if (message.sensitive === true) {
      writer.uint32(24).bool(message.sensitive)
    }
    return writer
  },
}

export const Log = {
  encode(message: Log, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.level !== 0) {
      writer.uint32(8).int32(message.level)
    }
    if (message.output !== "") {
      writer.uint32(18).string(message.output)
    }
    return writer
  },
}

export const InstanceIdentityAuth = {
  encode(
    message: InstanceIdentityAuth,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.instanceId !== "") {
      writer.uint32(10).string(message.instanceId)
    }
    return writer
  },
}

export const GitAuthProvider = {
  encode(
    message: GitAuthProvider,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id)
    }
    if (message.accessToken !== "") {
      writer.uint32(18).string(message.accessToken)
    }
    return writer
  },
}

export const Agent = {
  encode(message: Agent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id)
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name)
    }
    Object.entries(message.env).forEach(([key, value]) => {
      Agent_EnvEntry.encode(
        { key: key as any, value },
        writer.uint32(26).fork(),
      ).ldelim()
    })
    if (message.startupScript !== "") {
      writer.uint32(34).string(message.startupScript)
    }
    if (message.operatingSystem !== "") {
      writer.uint32(42).string(message.operatingSystem)
    }
    if (message.architecture !== "") {
      writer.uint32(50).string(message.architecture)
    }
    if (message.directory !== "") {
      writer.uint32(58).string(message.directory)
    }
    for (const v of message.apps) {
      App.encode(v!, writer.uint32(66).fork()).ldelim()
    }
    if (message.token !== undefined) {
      writer.uint32(74).string(message.token)
    }
    if (message.instanceId !== undefined) {
      writer.uint32(82).string(message.instanceId)
    }
    if (message.connectionTimeoutSeconds !== 0) {
      writer.uint32(88).int32(message.connectionTimeoutSeconds)
    }
    if (message.troubleshootingUrl !== "") {
      writer.uint32(98).string(message.troubleshootingUrl)
    }
    if (message.motdFile !== "") {
      writer.uint32(106).string(message.motdFile)
    }
    if (message.startupScriptTimeoutSeconds !== 0) {
      writer.uint32(120).int32(message.startupScriptTimeoutSeconds)
    }
    if (message.shutdownScript !== "") {
      writer.uint32(130).string(message.shutdownScript)
    }
    if (message.shutdownScriptTimeoutSeconds !== 0) {
      writer.uint32(136).int32(message.shutdownScriptTimeoutSeconds)
    }
    for (const v of message.metadata) {
      Agent_Metadata.encode(v!, writer.uint32(146).fork()).ldelim()
    }
    if (message.startupScriptBehavior !== "") {
      writer.uint32(154).string(message.startupScriptBehavior)
    }
    return writer
  },
}

export const Agent_Metadata = {
  encode(
    message: Agent_Metadata,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key)
    }
    if (message.displayName !== "") {
      writer.uint32(18).string(message.displayName)
    }
    if (message.script !== "") {
      writer.uint32(26).string(message.script)
    }
    if (message.interval !== 0) {
      writer.uint32(32).int64(message.interval)
    }
    if (message.timeout !== 0) {
      writer.uint32(40).int64(message.timeout)
    }
    return writer
  },
}

export const Agent_EnvEntry = {
  encode(
    message: Agent_EnvEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value)
    }
    return writer
  },
}

export const App = {
  encode(message: App, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.slug !== "") {
      writer.uint32(10).string(message.slug)
    }
    if (message.displayName !== "") {
      writer.uint32(18).string(message.displayName)
    }
    if (message.command !== "") {
      writer.uint32(26).string(message.command)
    }
    if (message.url !== "") {
      writer.uint32(34).string(message.url)
    }
    if (message.icon !== "") {
      writer.uint32(42).string(message.icon)
    }
    if (message.subdomain === true) {
      writer.uint32(48).bool(message.subdomain)
    }
    if (message.healthcheck !== undefined) {
      Healthcheck.encode(message.healthcheck, writer.uint32(58).fork()).ldelim()
    }
    if (message.sharingLevel !== 0) {
      writer.uint32(64).int32(message.sharingLevel)
    }
    if (message.external === true) {
      writer.uint32(72).bool(message.external)
    }
    return writer
  },
}

export const Healthcheck = {
  encode(
    message: Healthcheck,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.url !== "") {
      writer.uint32(10).string(message.url)
    }
    if (message.interval !== 0) {
      writer.uint32(16).int32(message.interval)
    }
    if (message.threshold !== 0) {
      writer.uint32(24).int32(message.threshold)
    }
    return writer
  },
}

export const Resource = {
  encode(
    message: Resource,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name)
    }
    if (message.type !== "") {
      writer.uint32(18).string(message.type)
    }
    for (const v of message.agents) {
      Agent.encode(v!, writer.uint32(26).fork()).ldelim()
    }
    for (const v of message.metadata) {
      Resource_Metadata.encode(v!, writer.uint32(34).fork()).ldelim()
    }
    if (message.hide === true) {
      writer.uint32(40).bool(message.hide)
    }
    if (message.icon !== "") {
      writer.uint32(50).string(message.icon)
    }
    if (message.instanceType !== "") {
      writer.uint32(58).string(message.instanceType)
    }
    if (message.dailyCost !== 0) {
      writer.uint32(64).int32(message.dailyCost)
    }
    return writer
  },
}

export const Resource_Metadata = {
  encode(
    message: Resource_Metadata,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value)
    }
    if (message.sensitive === true) {
      writer.uint32(24).bool(message.sensitive)
    }
    if (message.isNull === true) {
      writer.uint32(32).bool(message.isNull)
    }
    return writer
  },
}

export const Parse = {
  encode(_: Parse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer
  },
}

export const Parse_Request = {
  encode(
    message: Parse_Request,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.directory !== "") {
      writer.uint32(10).string(message.directory)
    }
    return writer
  },
}

export const Parse_Complete = {
  encode(
    message: Parse_Complete,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.templateVariables) {
      TemplateVariable.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },
}

export const Parse_Response = {
  encode(
    message: Parse_Response,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.log !== undefined) {
      Log.encode(message.log, writer.uint32(10).fork()).ldelim()
    }
    if (message.complete !== undefined) {
      Parse_Complete.encode(message.complete, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },
}

export const Provision = {
  encode(_: Provision, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer
  },
}

export const Provision_Metadata = {
  encode(
    message: Provision_Metadata,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.coderUrl !== "") {
      writer.uint32(10).string(message.coderUrl)
    }
    if (message.workspaceTransition !== 0) {
      writer.uint32(16).int32(message.workspaceTransition)
    }
    if (message.workspaceName !== "") {
      writer.uint32(26).string(message.workspaceName)
    }
    if (message.workspaceOwner !== "") {
      writer.uint32(34).string(message.workspaceOwner)
    }
    if (message.workspaceId !== "") {
      writer.uint32(42).string(message.workspaceId)
    }
    if (message.workspaceOwnerId !== "") {
      writer.uint32(50).string(message.workspaceOwnerId)
    }
    if (message.workspaceOwnerEmail !== "") {
      writer.uint32(58).string(message.workspaceOwnerEmail)
    }
    if (message.templateName !== "") {
      writer.uint32(66).string(message.templateName)
    }
    if (message.templateVersion !== "") {
      writer.uint32(74).string(message.templateVersion)
    }
    if (message.workspaceOwnerOidcAccessToken !== "") {
      writer.uint32(82).string(message.workspaceOwnerOidcAccessToken)
    }
    if (message.workspaceOwnerSessionToken !== "") {
      writer.uint32(90).string(message.workspaceOwnerSessionToken)
    }
    return writer
  },
}

export const Provision_Config = {
  encode(
    message: Provision_Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.directory !== "") {
      writer.uint32(10).string(message.directory)
    }
    if (message.state.length !== 0) {
      writer.uint32(18).bytes(message.state)
    }
    if (message.metadata !== undefined) {
      Provision_Metadata.encode(
        message.metadata,
        writer.uint32(26).fork(),
      ).ldelim()
    }
    if (message.provisionerLogLevel !== "") {
      writer.uint32(34).string(message.provisionerLogLevel)
    }
    return writer
  },
}

export const Provision_Plan = {
  encode(
    message: Provision_Plan,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.config !== undefined) {
      Provision_Config.encode(message.config, writer.uint32(10).fork()).ldelim()
    }
    for (const v of message.richParameterValues) {
      RichParameterValue.encode(v!, writer.uint32(26).fork()).ldelim()
    }
    for (const v of message.variableValues) {
      VariableValue.encode(v!, writer.uint32(34).fork()).ldelim()
    }
    for (const v of message.gitAuthProviders) {
      GitAuthProvider.encode(v!, writer.uint32(42).fork()).ldelim()
    }
    return writer
  },
}

export const Provision_Apply = {
  encode(
    message: Provision_Apply,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.config !== undefined) {
      Provision_Config.encode(message.config, writer.uint32(10).fork()).ldelim()
    }
    if (message.plan.length !== 0) {
      writer.uint32(18).bytes(message.plan)
    }
    return writer
  },
}

export const Provision_Cancel = {
  encode(
    _: Provision_Cancel,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },
}

export const Provision_Request = {
  encode(
    message: Provision_Request,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.plan !== undefined) {
      Provision_Plan.encode(message.plan, writer.uint32(10).fork()).ldelim()
    }
    if (message.apply !== undefined) {
      Provision_Apply.encode(message.apply, writer.uint32(18).fork()).ldelim()
    }
    if (message.cancel !== undefined) {
      Provision_Cancel.encode(message.cancel, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },
}

export const Provision_Complete = {
  encode(
    message: Provision_Complete,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.state.length !== 0) {
      writer.uint32(10).bytes(message.state)
    }
    if (message.error !== "") {
      writer.uint32(18).string(message.error)
    }
    for (const v of message.resources) {
      Resource.encode(v!, writer.uint32(26).fork()).ldelim()
    }
    for (const v of message.parameters) {
      RichParameter.encode(v!, writer.uint32(34).fork()).ldelim()
    }
    for (const v of message.gitAuthProviders) {
      writer.uint32(42).string(v!)
    }
    if (message.plan.length !== 0) {
      writer.uint32(50).bytes(message.plan)
    }
    return writer
  },
}

export const Provision_Response = {
  encode(
    message: Provision_Response,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.log !== undefined) {
      Log.encode(message.log, writer.uint32(10).fork()).ldelim()
    }
    if (message.complete !== undefined) {
      Provision_Complete.encode(
        message.complete,
        writer.uint32(18).fork(),
      ).ldelim()
    }
    return writer
  },
}

export interface Provisioner {
  Parse(request: Parse_Request): Observable<Parse_Response>
  Provision(
    request: Observable<Provision_Request>,
  ): Observable<Provision_Response>
}
