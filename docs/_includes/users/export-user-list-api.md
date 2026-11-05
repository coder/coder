Use [get users](../../reference/api/users.md#get-users):

```sh
curl -X GET http://coder-server:8080/api/v2/users \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

To export the results to a CSV file, you can use [`jq`](https://jqlang.org/) to process the JSON response:

```sh
curl -X GET http://coder-server:8080/api/v2/users \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY' | \
  jq -r '.users | (map(keys) | add | unique) as $cols | $cols, (.[] | [.[$cols[]]] | @csv)' > users.csv
```

Visit the [get users](../../reference/api/users.md#get-users) documentation for more options.
