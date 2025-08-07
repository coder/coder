# Default

## get__templates_{template}

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template} \
  -H 'Accept: */*'
```

`GET /templates/{template}`

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |
