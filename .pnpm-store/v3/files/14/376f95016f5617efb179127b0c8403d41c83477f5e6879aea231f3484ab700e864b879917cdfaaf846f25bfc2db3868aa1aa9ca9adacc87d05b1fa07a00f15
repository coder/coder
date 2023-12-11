'use strict';

const oas3_descs = {
  "infoObject": "The object provides metadata about the API. The metadata can be used by the clients if needed, and can be presented in editing or documentation generation tools for convenience.",
  "contactObject": "Contact information for the exposed API.",
  "licenseObject": "License information for the exposed API.",
  "serverObject": "An object representing a Server.",
  "serverVariablesObject": "",
  "serverVariableObject": "An object representing a Server Variable for server URL template substitution.",
  "componentsObject": "Holds a set of reusable objects for different aspects of the OAS. All objects defined within the components object will have no effect on the API unless they are explicitly referenced from properties outside the components object.",
  "pathsObject": "Holds the relative paths to the individual endpoints and their operations. The path is appended to the URL from the `Server Object` in order to construct the full URL.  The Paths MAY be empty, due to ACL constraints.",
  "pathItemObject": "Describes the operations available on a single path. A Path Item MAY be empty, due to ACL constraints. The path itself is still exposed to the documentation viewer but they will not know which operations and parameters are available.",
  "operationObject": "Describes a single API operation on a path.",
  "externalDocsObject": "Allows referencing an external resource for extended documentation.",
  "parameterObject": "Describes a single operation parameter.  A unique parameter is defined by a combination of a name and location.",
  "requestBodyObject": "Describes a single request body.",
  "contentObject": "Describes a set of supported media types. A Content Object can be used in Request Body Object, Parameter Objects, Header Objects, and Response Objects.",
  "mediaTypeObject": "Each Media Type Object provides schema and examples for a the media type identified by its key.  Media Type Objects can be used in a Content Object.",
  "encodingObject": "An object representing multipart region encoding for `requestBody` objects.",
  "encodingPropertyObject": "A single encoding definition applied to a single schema property.",
  "responsesObject": "A container for the expected responses of an operation. The container maps a HTTP response code to the expected response.  It is not expected for the documentation to necessarily cover all possible HTTP response codes, since they may not be known in advance. However, it is expected for the documentation to cover a successful operation response and any known errors.  The `default` MAY be used as a default response object for all HTTP codes  that are not covered individually by the specification.  The `Responses Object` MUST contain at least one response code, and it  SHOULD be the response for a successful operation call.",
  "responseObject": "Describes a single response from an API Operation, including design-time, static  `links` to operations based on the response.",
  "callbacksObject": "A map of possible out-of band callbacks related to the parent operation. Each value in the map is a Callback Object that describes a request that may be initiated by the API provider and the expected responses. The key value used to identify the callback object is an expression, evaluated at runtime, that identifies a URL to use for the callback operation.",
  "callbackObject": "A map of possible out-of band callbacks related to the parent operation. Each value in the map is a Path Item Object that describes a set of requests that may be initiated by the API provider and the expected responses. The key value used to identify the callback object is an expression, evaluated at runtime, that identifies a URL to use for the callback operation.",
  "headersObject": "Lists the headers that can be sent in a response or forwarded via a link. Note that RFC7230 states header names are case insensitive.",
  "exampleObject": "",
  "linksObject": "The links object represents a set of possible design-time links for a response. The presence of a link does not guarantee the caller's ability to successfully invoke it, rather it provides a known relationship and traversal mechanism between responses and other operations.  As opposed to _dynamic_ links (links provided **in** the response payload), the OAS linking mechanism does not require that link information be provided in a specific response format at runtime.  For computing links, and providing instructions to execute them, variable substitution is used for accessing values in a response and using them as values while invoking the linked operation.",
  "linkObject": "The `Link Object` is responsible for defining a possible operation based on a single response.",
  "linkParametersObject": "Using the `operationId` to reference an operation in the definition has many benefits, including the ability to define media type options, security requirements, response and error payloads. Many operations require parameters to be passed, and these MAY be dynamic depending on the response itself.  To specify parameters required by the operation, we can use a **Link Parameters Object**. This object contains parameter names along with static or dynamic values:",
  "headerObject": "The Header Object follows the structure of the Parameter Object, with the following changes:  1. `name` MUST NOT be specified, it is given in the Headers Object. 1. `in` MUST NOT be specified, it is implicitly in `header`. 1. All traits that are affected by the location MUST be applicable to a location of `header` (for example, `style`).",
  "tagObject": "Allows adding meta data to a single tag that is used by the Operation Object. It is not mandatory to have a Tag Object per tag used there.",
  "examplesObject": "",
  "referenceObject": "A simple object to allow referencing other components in the specification, internally and externally.  The Reference Object is defined by JSON Reference and follows the same structure, behavior and rules.   For this specification, reference resolution is done as defined by the JSON Reference specification and not by the JSON Schema specification.",
  "schemaObject": "The Schema Object allows the definition of input and output data types. These types can be objects, but also primitives and arrays. This object is an extended subset of the JSON Schema Specification Wright Draft 00.  Further information about the properties can be found in JSON Schema Core and JSON Schema Validation. Unless stated otherwise, the property definitions follow the JSON Schema specification as referenced here.",
  "discriminatorObject": "When request bodies or response payloads may be one of a number of different schemas, a `discriminator` object can be used to aid in serialization, deserialization, and validation.  The discriminator is a specific object in a schema which is used to inform the consumer of the specification of an alternative schema based on the value associated with it.  Note, when using the discriminator, _inline_ schemas will not be considered when using the discriminator.",
  "xmlObject": "A metadata object that allows for more fine-tuned XML model definitions.  When using arrays, XML element names are *not* inferred (for singular/plural forms) and the `name` property SHOULD be used to add that information. See examples for expected behavior.",
  "securitySchemeObject": "Allows the definition of a security scheme that can be used by the operations. Supported schemes are HTTP authentication, an API key (either as a header or as a query parameter) and OAuth2's common flows (implicit, password, application and access code).",
  "oauthFlowsObject": "Allows configuration of the supported OAuth Flows.",
  "oauthFlowObject": "Configuration details for a supported OAuth Flow",
  "scopesObject": "Lists the available scopes for an OAuth2 security scheme.",
  "securityRequirementObject": "Lists the required security schemes to execute this operation. The name used for each property MUST correspond to a security scheme declared in the Security Schemes under the Components Object.  Security Requirement Objects that contain multiple schemes require that all schemes MUST be satisfied for a request to be authorized. This enables support for scenarios where there multiple query parameters or HTTP headers are required to convey security information.  When a list of Security Requirement Objects is defined on the Open API object or Operation Object, only one of Security Requirement Objects in the list needs to be satisfied to authorize.",
  "specificationExtensionObject": "Any property starting with x- is valid."
}

module.exports = {
    oas2_descs : oas3_descs,
    oas3_descs : oas3_descs
};

