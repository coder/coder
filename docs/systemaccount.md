# SystemAccount

Providing a system account where user credentials are not longer needed for a web application has several advantages, including:
1. Improved security: With a system account, the web application is not dependent on user credentials for access. This can reduce the risk of data breaches caused by weak or compromised user passwords.
2. Easier management: By using a system account, administrators can manage access to the web application more easily. For example, they can easily revoke or update permissions for the account without affecting individual user accounts.
3. Better scalability: A system account can handle multiple requests simultaneously, which can improve the overall performance and scalability of the web application.
4. Enhanced automation: A system account can be used to automate tasks within the web application, such as running scripts or performing batch operations. This can help streamline workflows and reduce the need for manual intervention.

# SystemAccount
The System Account API provides the following endpoints:

1. Create System Account
This endpoint is used to create a new system account. The request must include the following parameters:

name (string, required): The name of the new system account.

```
POST /systemaccounts
{
    "name": "My System Account"
}

Example Response
HTTP/1.1 201 OK
{
    "name": "My System Account",
    "id": "8e8e796c-ba97-475c-936c-e129b8a03d18",
    "created_at": "2023-04-07T12:34:56Z",
    "updated_at": "2023-04-07T12:34:56Z",
    "organization_id": "8e8e796c-ba97-475c-936c-e129b8a0ap32",
    "created_by": <UserID of the user who made this request, should be a owner>
}

```

2. Update System Account
This endpoint is used to update an existing system account by ID. The request must include the following parameters:

name (string, optional): The new name of the system account.

```
PUT /systemaccounts/8e8e796c-ba97-475c-936c-e129b8a03d18
{
    "name": "My Updated System Account"
}

HTTP/1.1 200 OK

{
    "name": "My Updated System Account",
    "id": "8e8e796c-ba97-475c-936c-e129b8a03d18",
    "created_at": "2023-04-07T12:34:56Z",
    "updated_at": "2023-04-08T22:45:12Z",
    "organization_id": "8e8e796c-ba97-475c-936c-e129b8a0ap32",
    "created_by": <UserID of the user who made this request, should be a owner>
}

```

3. Delete System Account
Deletes an existing system account from the database. The account is identified by the provided id parameter.

```

DELETE /systemaccounts/{id}

HTTP/1.1 204 OK


```

# System Account Token API

4. Create System Account Token
Create a new JWT token for a given system account ID.

```
POST /api/v2/systemaccounts/{id}/tokens

HTTP/1.1 200 OK

{
    "result": {
        "token": "<token>"
    }
}

```

5. Invalidate System Account Token
Invalidate a previously created JWT token for a given system account ID.

```
DELETE /api/v2/systemaccounts/{id}/tokens/{tokenid}

ex:
DELETE /api/v2/systemaccounts/12345/tokens/67890 HTTP/1.1
Authorization: Bearer <session_token>

{
    "id": "12345",
    "tokenid": "67890"
}

HTTP/1.1 204 OK
{}
```


# Data Model

1. CREATE TABLE system_account (
    id SERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL,
    expiration_time TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);


2. CREATE TABLE organization_systemaccounts (
  id SERIAL PRIMARY KEY,
  organization_id INTEGER NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  system_account_id INTEGER NOT NULL REFERENCES system_accounts (id) ON DELETE CASCADE,
  expiration_time TIMESTAMP NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  UNIQUE (organization_id, system_account_id)
);


Note: 
1. Could not create migration, so was unable to create queries and use database models to separate apis and Database transaction queries


Improvements:
1. Implementation of authentication of token, implementation of roles and resources that can be accessed through system Account token
2. Adding data table models and generating db queries and utilizing tools to securely access data
3. Error Handling and returning Api errors 




