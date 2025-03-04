/* eslint-disable */
import * as _m0 from "protobufjs/minimal";
import { Observable } from "rxjs";
import { Timestamp } from "./google/protobuf/timestampGenerated";

export const protobufPackage = "provisioner";

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

export enum AppOpenIn {
  /** @deprecated */
  WINDOW = 0,
  SLIM_WINDOW = 1,
  TAB = 2,
  UNRECOGNIZED = -1,
}

/** WorkspaceTransition is the desired outcome of a build */
export enum WorkspaceTransition {
  START = 0,
  STOP = 1,
  DESTROY = 2,
  UNRECOGNIZED = -1,
}

export enum TimingState {
  STARTED = 0,
  COMPLETED = 1,
  FAILED = 2,
  UNRECOGNIZED = -1,
}

/** Empty indicates a successful request/response. */
export interface Empty {
}

/** TemplateVariable represents a Terraform variable. */
export interface TemplateVariable {
  name: string;
  description: string;
  type: string;
  defaultValue: string;
  required: boolean;
  sensitive: boolean;
}

/** RichParameterOption represents a singular option that a parameter may expose. */
export interface RichParameterOption {
  name: string;
  description: string;
  value: string;
  icon: string;
}

/** RichParameter represents a variable that is exposed. */
export interface RichParameter {
  name: string;
  description: string;
  type: string;
  mutable: boolean;
  defaultValue: string;
  icon: string;
  options: RichParameterOption[];
  validationRegex: string;
  validationError: string;
  validationMin?: number | undefined;
  validationMax?: number | undefined;
  validationMonotonic: string;
  required: boolean;
  /** legacy_variable_name was removed (= 14) */
  displayName: string;
  order: number;
  ephemeral: boolean;
}

/** RichParameterValue holds the key/value mapping of a parameter. */
export interface RichParameterValue {
  name: string;
  value: string;
}

export interface Prebuild {
  instances: number;
}

/** Preset represents a set of preset parameters for a template version. */
export interface Preset {
  name: string;
  parameters: PresetParameter[];
  prebuild: Prebuild | undefined;
}

export interface PresetParameter {
  name: string;
  value: string;
}

/** VariableValue holds the key/value mapping of a Terraform variable. */
export interface VariableValue {
  name: string;
  value: string;
  sensitive: boolean;
}

/** Log represents output from a request. */
export interface Log {
  level: LogLevel;
  output: string;
}

export interface InstanceIdentityAuth {
  instanceId: string;
}

export interface ExternalAuthProviderResource {
  id: string;
  optional: boolean;
}

export interface ExternalAuthProvider {
  id: string;
  accessToken: string;
}

/** Agent represents a running agent on the workspace. */
export interface Agent {
  id: string;
  name: string;
  env: { [key: string]: string };
  /** Field 4 was startup_script, now removed. */
  operatingSystem: string;
  architecture: string;
  directory: string;
  apps: App[];
  token?: string | undefined;
  instanceId?: string | undefined;
  connectionTimeoutSeconds: number;
  troubleshootingUrl: string;
  motdFile: string;
  /**
   * Field 14 was bool login_before_ready = 14, now removed.
   * Field 15, 16, 17 were related to scripts, which are now removed.
   */
  metadata: Agent_Metadata[];
  /** Field 19 was startup_script_behavior, now removed. */
  displayApps: DisplayApps | undefined;
  scripts: Script[];
  extraEnvs: Env[];
  order: number;
  resourcesMonitoring: ResourcesMonitoring | undefined;
}

export interface Agent_Metadata {
  key: string;
  displayName: string;
  script: string;
  interval: number;
  timeout: number;
  order: number;
}

export interface Agent_EnvEntry {
  key: string;
  value: string;
}

export interface ResourcesMonitoring {
  memory: MemoryResourceMonitor | undefined;
  volumes: VolumeResourceMonitor[];
}

export interface MemoryResourceMonitor {
  enabled: boolean;
  threshold: number;
}

export interface VolumeResourceMonitor {
  path: string;
  enabled: boolean;
  threshold: number;
}

export interface DisplayApps {
  vscode: boolean;
  vscodeInsiders: boolean;
  webTerminal: boolean;
  sshHelper: boolean;
  portForwardingHelper: boolean;
}

export interface Env {
  name: string;
  value: string;
}

/** Script represents a script to be run on the workspace. */
export interface Script {
  displayName: string;
  icon: string;
  script: string;
  cron: string;
  startBlocksLogin: boolean;
  runOnStart: boolean;
  runOnStop: boolean;
  timeoutSeconds: number;
  logPath: string;
}

/** App represents a dev-accessible application on the workspace. */
export interface App {
  /**
   * slug is the unique identifier for the app, usually the name from the
   * template. It must be URL-safe and hostname-safe.
   */
  slug: string;
  displayName: string;
  command: string;
  url: string;
  icon: string;
  subdomain: boolean;
  healthcheck: Healthcheck | undefined;
  sharingLevel: AppSharingLevel;
  external: boolean;
  order: number;
  hidden: boolean;
  openIn: AppOpenIn;
}

/** Healthcheck represents configuration for checking for app readiness. */
export interface Healthcheck {
  url: string;
  interval: number;
  threshold: number;
}

/** Resource represents created infrastructure. */
export interface Resource {
  name: string;
  type: string;
  agents: Agent[];
  metadata: Resource_Metadata[];
  hide: boolean;
  icon: string;
  instanceType: string;
  dailyCost: number;
  modulePath: string;
}

export interface Resource_Metadata {
  key: string;
  value: string;
  sensitive: boolean;
  isNull: boolean;
}

export interface Module {
  source: string;
  version: string;
  key: string;
}

export interface Role {
  name: string;
  orgId: string;
}

/** Metadata is information about a workspace used in the execution of a build */
export interface Metadata {
  coderUrl: string;
  workspaceTransition: WorkspaceTransition;
  workspaceName: string;
  workspaceOwner: string;
  workspaceId: string;
  workspaceOwnerId: string;
  workspaceOwnerEmail: string;
  templateName: string;
  templateVersion: string;
  workspaceOwnerOidcAccessToken: string;
  workspaceOwnerSessionToken: string;
  templateId: string;
  workspaceOwnerName: string;
  workspaceOwnerGroups: string[];
  workspaceOwnerSshPublicKey: string;
  workspaceOwnerSshPrivateKey: string;
  workspaceBuildId: string;
  workspaceOwnerLoginType: string;
  workspaceOwnerRbacRoles: Role[];
  isPrebuild: boolean;
  runningWorkspaceAgentToken: string;
}

/** Config represents execution configuration shared by all subsequent requests in the Session */
export interface Config {
  /** template_source_archive is a tar of the template source files */
  templateSourceArchive: Uint8Array;
  /** state is the provisioner state (if any) */
  state: Uint8Array;
  provisionerLogLevel: string;
}

/** ParseRequest consumes source-code to produce inputs. */
export interface ParseRequest {
}

/** ParseComplete indicates a request to parse completed. */
export interface ParseComplete {
  error: string;
  templateVariables: TemplateVariable[];
  readme: Uint8Array;
  workspaceTags: { [key: string]: string };
}

export interface ParseComplete_WorkspaceTagsEntry {
  key: string;
  value: string;
}

/** PlanRequest asks the provisioner to plan what resources & parameters it will create */
export interface PlanRequest {
  metadata: Metadata | undefined;
  richParameterValues: RichParameterValue[];
  variableValues: VariableValue[];
  externalAuthProviders: ExternalAuthProvider[];
}

/** PlanComplete indicates a request to plan completed. */
export interface PlanComplete {
  error: string;
  resources: Resource[];
  parameters: RichParameter[];
  externalAuthProviders: ExternalAuthProviderResource[];
  timings: Timing[];
  modules: Module[];
  presets: Preset[];
}

/**
 * ApplyRequest asks the provisioner to apply the changes.  Apply MUST be preceded by a successful plan request/response
 * in the same Session.  The plan data is not transmitted over the wire and is cached by the provisioner in the Session.
 */
export interface ApplyRequest {
  metadata: Metadata | undefined;
}

/** ApplyComplete indicates a request to apply completed. */
export interface ApplyComplete {
  state: Uint8Array;
  error: string;
  resources: Resource[];
  parameters: RichParameter[];
  externalAuthProviders: ExternalAuthProviderResource[];
  timings: Timing[];
}

export interface Timing {
  start: Date | undefined;
  end: Date | undefined;
  action: string;
  source: string;
  resource: string;
  stage: string;
  state: TimingState;
}

/** CancelRequest requests that the previous request be canceled gracefully. */
export interface CancelRequest {
}

export interface Request {
  config?: Config | undefined;
  parse?: ParseRequest | undefined;
  plan?: PlanRequest | undefined;
  apply?: ApplyRequest | undefined;
  cancel?: CancelRequest | undefined;
}

export interface Response {
  log?: Log | undefined;
  parse?: ParseComplete | undefined;
  plan?: PlanComplete | undefined;
  apply?: ApplyComplete | undefined;
}

export const Empty = {
  encode(_: Empty, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },
};

export const TemplateVariable = {
  encode(message: TemplateVariable, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.description !== "") {
      writer.uint32(18).string(message.description);
    }
    if (message.type !== "") {
      writer.uint32(26).string(message.type);
    }
    if (message.defaultValue !== "") {
      writer.uint32(34).string(message.defaultValue);
    }
    if (message.required === true) {
      writer.uint32(40).bool(message.required);
    }
    if (message.sensitive === true) {
      writer.uint32(48).bool(message.sensitive);
    }
    return writer;
  },
};

export const RichParameterOption = {
  encode(message: RichParameterOption, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.description !== "") {
      writer.uint32(18).string(message.description);
    }
    if (message.value !== "") {
      writer.uint32(26).string(message.value);
    }
    if (message.icon !== "") {
      writer.uint32(34).string(message.icon);
    }
    return writer;
  },
};

export const RichParameter = {
  encode(message: RichParameter, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.description !== "") {
      writer.uint32(18).string(message.description);
    }
    if (message.type !== "") {
      writer.uint32(26).string(message.type);
    }
    if (message.mutable === true) {
      writer.uint32(32).bool(message.mutable);
    }
    if (message.defaultValue !== "") {
      writer.uint32(42).string(message.defaultValue);
    }
    if (message.icon !== "") {
      writer.uint32(50).string(message.icon);
    }
    for (const v of message.options) {
      RichParameterOption.encode(v!, writer.uint32(58).fork()).ldelim();
    }
    if (message.validationRegex !== "") {
      writer.uint32(66).string(message.validationRegex);
    }
    if (message.validationError !== "") {
      writer.uint32(74).string(message.validationError);
    }
    if (message.validationMin !== undefined) {
      writer.uint32(80).int32(message.validationMin);
    }
    if (message.validationMax !== undefined) {
      writer.uint32(88).int32(message.validationMax);
    }
    if (message.validationMonotonic !== "") {
      writer.uint32(98).string(message.validationMonotonic);
    }
    if (message.required === true) {
      writer.uint32(104).bool(message.required);
    }
    if (message.displayName !== "") {
      writer.uint32(122).string(message.displayName);
    }
    if (message.order !== 0) {
      writer.uint32(128).int32(message.order);
    }
    if (message.ephemeral === true) {
      writer.uint32(136).bool(message.ephemeral);
    }
    return writer;
  },
};

export const RichParameterValue = {
  encode(message: RichParameterValue, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },
};

export const Prebuild = {
  encode(message: Prebuild, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.instances !== 0) {
      writer.uint32(8).int32(message.instances);
    }
    return writer;
  },
};

export const Preset = {
  encode(message: Preset, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    for (const v of message.parameters) {
      PresetParameter.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.prebuild !== undefined) {
      Prebuild.encode(message.prebuild, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },
};

export const PresetParameter = {
  encode(message: PresetParameter, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },
};

export const VariableValue = {
  encode(message: VariableValue, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    if (message.sensitive === true) {
      writer.uint32(24).bool(message.sensitive);
    }
    return writer;
  },
};

export const Log = {
  encode(message: Log, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.level !== 0) {
      writer.uint32(8).int32(message.level);
    }
    if (message.output !== "") {
      writer.uint32(18).string(message.output);
    }
    return writer;
  },
};

export const InstanceIdentityAuth = {
  encode(message: InstanceIdentityAuth, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.instanceId !== "") {
      writer.uint32(10).string(message.instanceId);
    }
    return writer;
  },
};

export const ExternalAuthProviderResource = {
  encode(message: ExternalAuthProviderResource, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.optional === true) {
      writer.uint32(16).bool(message.optional);
    }
    return writer;
  },
};

export const ExternalAuthProvider = {
  encode(message: ExternalAuthProvider, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.accessToken !== "") {
      writer.uint32(18).string(message.accessToken);
    }
    return writer;
  },
};

export const Agent = {
  encode(message: Agent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    Object.entries(message.env).forEach(([key, value]) => {
      Agent_EnvEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    if (message.operatingSystem !== "") {
      writer.uint32(42).string(message.operatingSystem);
    }
    if (message.architecture !== "") {
      writer.uint32(50).string(message.architecture);
    }
    if (message.directory !== "") {
      writer.uint32(58).string(message.directory);
    }
    for (const v of message.apps) {
      App.encode(v!, writer.uint32(66).fork()).ldelim();
    }
    if (message.token !== undefined) {
      writer.uint32(74).string(message.token);
    }
    if (message.instanceId !== undefined) {
      writer.uint32(82).string(message.instanceId);
    }
    if (message.connectionTimeoutSeconds !== 0) {
      writer.uint32(88).int32(message.connectionTimeoutSeconds);
    }
    if (message.troubleshootingUrl !== "") {
      writer.uint32(98).string(message.troubleshootingUrl);
    }
    if (message.motdFile !== "") {
      writer.uint32(106).string(message.motdFile);
    }
    for (const v of message.metadata) {
      Agent_Metadata.encode(v!, writer.uint32(146).fork()).ldelim();
    }
    if (message.displayApps !== undefined) {
      DisplayApps.encode(message.displayApps, writer.uint32(162).fork()).ldelim();
    }
    for (const v of message.scripts) {
      Script.encode(v!, writer.uint32(170).fork()).ldelim();
    }
    for (const v of message.extraEnvs) {
      Env.encode(v!, writer.uint32(178).fork()).ldelim();
    }
    if (message.order !== 0) {
      writer.uint32(184).int64(message.order);
    }
    if (message.resourcesMonitoring !== undefined) {
      ResourcesMonitoring.encode(message.resourcesMonitoring, writer.uint32(194).fork()).ldelim();
    }
    return writer;
  },
};

export const Agent_Metadata = {
  encode(message: Agent_Metadata, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.displayName !== "") {
      writer.uint32(18).string(message.displayName);
    }
    if (message.script !== "") {
      writer.uint32(26).string(message.script);
    }
    if (message.interval !== 0) {
      writer.uint32(32).int64(message.interval);
    }
    if (message.timeout !== 0) {
      writer.uint32(40).int64(message.timeout);
    }
    if (message.order !== 0) {
      writer.uint32(48).int64(message.order);
    }
    return writer;
  },
};

export const Agent_EnvEntry = {
  encode(message: Agent_EnvEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },
};

export const ResourcesMonitoring = {
  encode(message: ResourcesMonitoring, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.memory !== undefined) {
      MemoryResourceMonitor.encode(message.memory, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.volumes) {
      VolumeResourceMonitor.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },
};

export const MemoryResourceMonitor = {
  encode(message: MemoryResourceMonitor, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.enabled === true) {
      writer.uint32(8).bool(message.enabled);
    }
    if (message.threshold !== 0) {
      writer.uint32(16).int32(message.threshold);
    }
    return writer;
  },
};

export const VolumeResourceMonitor = {
  encode(message: VolumeResourceMonitor, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.path !== "") {
      writer.uint32(10).string(message.path);
    }
    if (message.enabled === true) {
      writer.uint32(16).bool(message.enabled);
    }
    if (message.threshold !== 0) {
      writer.uint32(24).int32(message.threshold);
    }
    return writer;
  },
};

export const DisplayApps = {
  encode(message: DisplayApps, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.vscode === true) {
      writer.uint32(8).bool(message.vscode);
    }
    if (message.vscodeInsiders === true) {
      writer.uint32(16).bool(message.vscodeInsiders);
    }
    if (message.webTerminal === true) {
      writer.uint32(24).bool(message.webTerminal);
    }
    if (message.sshHelper === true) {
      writer.uint32(32).bool(message.sshHelper);
    }
    if (message.portForwardingHelper === true) {
      writer.uint32(40).bool(message.portForwardingHelper);
    }
    return writer;
  },
};

export const Env = {
  encode(message: Env, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },
};

export const Script = {
  encode(message: Script, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.displayName !== "") {
      writer.uint32(10).string(message.displayName);
    }
    if (message.icon !== "") {
      writer.uint32(18).string(message.icon);
    }
    if (message.script !== "") {
      writer.uint32(26).string(message.script);
    }
    if (message.cron !== "") {
      writer.uint32(34).string(message.cron);
    }
    if (message.startBlocksLogin === true) {
      writer.uint32(40).bool(message.startBlocksLogin);
    }
    if (message.runOnStart === true) {
      writer.uint32(48).bool(message.runOnStart);
    }
    if (message.runOnStop === true) {
      writer.uint32(56).bool(message.runOnStop);
    }
    if (message.timeoutSeconds !== 0) {
      writer.uint32(64).int32(message.timeoutSeconds);
    }
    if (message.logPath !== "") {
      writer.uint32(74).string(message.logPath);
    }
    return writer;
  },
};

export const App = {
  encode(message: App, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.slug !== "") {
      writer.uint32(10).string(message.slug);
    }
    if (message.displayName !== "") {
      writer.uint32(18).string(message.displayName);
    }
    if (message.command !== "") {
      writer.uint32(26).string(message.command);
    }
    if (message.url !== "") {
      writer.uint32(34).string(message.url);
    }
    if (message.icon !== "") {
      writer.uint32(42).string(message.icon);
    }
    if (message.subdomain === true) {
      writer.uint32(48).bool(message.subdomain);
    }
    if (message.healthcheck !== undefined) {
      Healthcheck.encode(message.healthcheck, writer.uint32(58).fork()).ldelim();
    }
    if (message.sharingLevel !== 0) {
      writer.uint32(64).int32(message.sharingLevel);
    }
    if (message.external === true) {
      writer.uint32(72).bool(message.external);
    }
    if (message.order !== 0) {
      writer.uint32(80).int64(message.order);
    }
    if (message.hidden === true) {
      writer.uint32(88).bool(message.hidden);
    }
    if (message.openIn !== 0) {
      writer.uint32(96).int32(message.openIn);
    }
    return writer;
  },
};

export const Healthcheck = {
  encode(message: Healthcheck, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.url !== "") {
      writer.uint32(10).string(message.url);
    }
    if (message.interval !== 0) {
      writer.uint32(16).int32(message.interval);
    }
    if (message.threshold !== 0) {
      writer.uint32(24).int32(message.threshold);
    }
    return writer;
  },
};

export const Resource = {
  encode(message: Resource, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.type !== "") {
      writer.uint32(18).string(message.type);
    }
    for (const v of message.agents) {
      Agent.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.metadata) {
      Resource_Metadata.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    if (message.hide === true) {
      writer.uint32(40).bool(message.hide);
    }
    if (message.icon !== "") {
      writer.uint32(50).string(message.icon);
    }
    if (message.instanceType !== "") {
      writer.uint32(58).string(message.instanceType);
    }
    if (message.dailyCost !== 0) {
      writer.uint32(64).int32(message.dailyCost);
    }
    if (message.modulePath !== "") {
      writer.uint32(74).string(message.modulePath);
    }
    return writer;
  },
};

export const Resource_Metadata = {
  encode(message: Resource_Metadata, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    if (message.sensitive === true) {
      writer.uint32(24).bool(message.sensitive);
    }
    if (message.isNull === true) {
      writer.uint32(32).bool(message.isNull);
    }
    return writer;
  },
};

export const Module = {
  encode(message: Module, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.source !== "") {
      writer.uint32(10).string(message.source);
    }
    if (message.version !== "") {
      writer.uint32(18).string(message.version);
    }
    if (message.key !== "") {
      writer.uint32(26).string(message.key);
    }
    return writer;
  },
};

export const Role = {
  encode(message: Role, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.orgId !== "") {
      writer.uint32(18).string(message.orgId);
    }
    return writer;
  },
};

export const Metadata = {
  encode(message: Metadata, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.coderUrl !== "") {
      writer.uint32(10).string(message.coderUrl);
    }
    if (message.workspaceTransition !== 0) {
      writer.uint32(16).int32(message.workspaceTransition);
    }
    if (message.workspaceName !== "") {
      writer.uint32(26).string(message.workspaceName);
    }
    if (message.workspaceOwner !== "") {
      writer.uint32(34).string(message.workspaceOwner);
    }
    if (message.workspaceId !== "") {
      writer.uint32(42).string(message.workspaceId);
    }
    if (message.workspaceOwnerId !== "") {
      writer.uint32(50).string(message.workspaceOwnerId);
    }
    if (message.workspaceOwnerEmail !== "") {
      writer.uint32(58).string(message.workspaceOwnerEmail);
    }
    if (message.templateName !== "") {
      writer.uint32(66).string(message.templateName);
    }
    if (message.templateVersion !== "") {
      writer.uint32(74).string(message.templateVersion);
    }
    if (message.workspaceOwnerOidcAccessToken !== "") {
      writer.uint32(82).string(message.workspaceOwnerOidcAccessToken);
    }
    if (message.workspaceOwnerSessionToken !== "") {
      writer.uint32(90).string(message.workspaceOwnerSessionToken);
    }
    if (message.templateId !== "") {
      writer.uint32(98).string(message.templateId);
    }
    if (message.workspaceOwnerName !== "") {
      writer.uint32(106).string(message.workspaceOwnerName);
    }
    for (const v of message.workspaceOwnerGroups) {
      writer.uint32(114).string(v!);
    }
    if (message.workspaceOwnerSshPublicKey !== "") {
      writer.uint32(122).string(message.workspaceOwnerSshPublicKey);
    }
    if (message.workspaceOwnerSshPrivateKey !== "") {
      writer.uint32(130).string(message.workspaceOwnerSshPrivateKey);
    }
    if (message.workspaceBuildId !== "") {
      writer.uint32(138).string(message.workspaceBuildId);
    }
    if (message.workspaceOwnerLoginType !== "") {
      writer.uint32(146).string(message.workspaceOwnerLoginType);
    }
    for (const v of message.workspaceOwnerRbacRoles) {
      Role.encode(v!, writer.uint32(154).fork()).ldelim();
    }
    if (message.isPrebuild === true) {
      writer.uint32(160).bool(message.isPrebuild);
    }
    if (message.runningWorkspaceAgentToken !== "") {
      writer.uint32(170).string(message.runningWorkspaceAgentToken);
    }
    return writer;
  },
};

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.templateSourceArchive.length !== 0) {
      writer.uint32(10).bytes(message.templateSourceArchive);
    }
    if (message.state.length !== 0) {
      writer.uint32(18).bytes(message.state);
    }
    if (message.provisionerLogLevel !== "") {
      writer.uint32(26).string(message.provisionerLogLevel);
    }
    return writer;
  },
};

export const ParseRequest = {
  encode(_: ParseRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },
};

export const ParseComplete = {
  encode(message: ParseComplete, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    for (const v of message.templateVariables) {
      TemplateVariable.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.readme.length !== 0) {
      writer.uint32(26).bytes(message.readme);
    }
    Object.entries(message.workspaceTags).forEach(([key, value]) => {
      ParseComplete_WorkspaceTagsEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
    });
    return writer;
  },
};

export const ParseComplete_WorkspaceTagsEntry = {
  encode(message: ParseComplete_WorkspaceTagsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },
};

export const PlanRequest = {
  encode(message: PlanRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.metadata !== undefined) {
      Metadata.encode(message.metadata, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.richParameterValues) {
      RichParameterValue.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.variableValues) {
      VariableValue.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.externalAuthProviders) {
      ExternalAuthProvider.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },
};

export const PlanComplete = {
  encode(message: PlanComplete, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.error !== "") {
      writer.uint32(10).string(message.error);
    }
    for (const v of message.resources) {
      Resource.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.parameters) {
      RichParameter.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.externalAuthProviders) {
      ExternalAuthProviderResource.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    for (const v of message.timings) {
      Timing.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    for (const v of message.modules) {
      Module.encode(v!, writer.uint32(58).fork()).ldelim();
    }
    for (const v of message.presets) {
      Preset.encode(v!, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },
};

export const ApplyRequest = {
  encode(message: ApplyRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.metadata !== undefined) {
      Metadata.encode(message.metadata, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },
};

export const ApplyComplete = {
  encode(message: ApplyComplete, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.state.length !== 0) {
      writer.uint32(10).bytes(message.state);
    }
    if (message.error !== "") {
      writer.uint32(18).string(message.error);
    }
    for (const v of message.resources) {
      Resource.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.parameters) {
      RichParameter.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    for (const v of message.externalAuthProviders) {
      ExternalAuthProviderResource.encode(v!, writer.uint32(42).fork()).ldelim();
    }
    for (const v of message.timings) {
      Timing.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },
};

export const Timing = {
  encode(message: Timing, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.start !== undefined) {
      Timestamp.encode(toTimestamp(message.start), writer.uint32(10).fork()).ldelim();
    }
    if (message.end !== undefined) {
      Timestamp.encode(toTimestamp(message.end), writer.uint32(18).fork()).ldelim();
    }
    if (message.action !== "") {
      writer.uint32(26).string(message.action);
    }
    if (message.source !== "") {
      writer.uint32(34).string(message.source);
    }
    if (message.resource !== "") {
      writer.uint32(42).string(message.resource);
    }
    if (message.stage !== "") {
      writer.uint32(50).string(message.stage);
    }
    if (message.state !== 0) {
      writer.uint32(56).int32(message.state);
    }
    return writer;
  },
};

export const CancelRequest = {
  encode(_: CancelRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },
};

export const Request = {
  encode(message: Request, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.config !== undefined) {
      Config.encode(message.config, writer.uint32(10).fork()).ldelim();
    }
    if (message.parse !== undefined) {
      ParseRequest.encode(message.parse, writer.uint32(18).fork()).ldelim();
    }
    if (message.plan !== undefined) {
      PlanRequest.encode(message.plan, writer.uint32(26).fork()).ldelim();
    }
    if (message.apply !== undefined) {
      ApplyRequest.encode(message.apply, writer.uint32(34).fork()).ldelim();
    }
    if (message.cancel !== undefined) {
      CancelRequest.encode(message.cancel, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },
};

export const Response = {
  encode(message: Response, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.log !== undefined) {
      Log.encode(message.log, writer.uint32(10).fork()).ldelim();
    }
    if (message.parse !== undefined) {
      ParseComplete.encode(message.parse, writer.uint32(18).fork()).ldelim();
    }
    if (message.plan !== undefined) {
      PlanComplete.encode(message.plan, writer.uint32(26).fork()).ldelim();
    }
    if (message.apply !== undefined) {
      ApplyComplete.encode(message.apply, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },
};

export interface Provisioner {
  /**
   * Session represents provisioning a single template import or workspace.  The daemon always sends Config followed
   * by one of the requests (ParseRequest, PlanRequest, ApplyRequest).  The provisioner should respond with a stream
   * of zero or more Logs, followed by the corresponding complete message (ParseComplete, PlanComplete,
   * ApplyComplete).  The daemon may then send a new request.  A request to apply MUST be preceded by a request plan,
   * and the provisioner should store the plan data on the Session after a successful plan, so that the daemon may
   * request an apply.  If the daemon closes the Session without an apply, the plan data may be safely discarded.
   *
   * The daemon may send a CancelRequest, asynchronously to ask the provisioner to cancel the previous ParseRequest,
   * PlanRequest, or ApplyRequest.  The provisioner MUST reply with a complete message corresponding to the request
   * that was canceled.  If the provisioner has already completed the request, it may ignore the CancelRequest.
   */
  Session(request: Observable<Request>): Observable<Response>;
}

function toTimestamp(date: Date): Timestamp {
  const seconds = date.getTime() / 1_000;
  const nanos = (date.getTime() % 1_000) * 1_000_000;
  return { seconds, nanos };
}
