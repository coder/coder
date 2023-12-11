## AsyncAPI 1.x template parameters

### Code templates

* `topic` - the current topic
* `message` - the current message
* `resource` - the current tag/topic object
* `tags[]` - the full list of tags applying to the message
* `payload` - containing `.obj`, `.str` and `.json` properties
* `header` - containing `.obj`, `.str` and `.json` properties

### Parameter Object (AsyncAPI 1.1+)

* After `data.utils.getParameters` is called
    * `name` - the name of the parameter
    * `in` - always `topic` in AsyncAPI v1.1
    * `required` - boolean. Defaulted to `true`
    * `safeType` - should usually be `string`
    * `shortDesc` - the parameter description

### Payload template

As above for code templates

### Header templates

As above for code templates

### Common to all templates

* `api` - the top-level AsyncAPI document
* `header` - the front-matter of the Slate/Shins markdown document
* `servers` - the (computed) servers of the API
* `baseTopic` - the baseTopic of the API
* `contactName` - the (possibly default) contact name for the API
