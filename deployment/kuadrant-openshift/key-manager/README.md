# Key Manager Quickstart

Quickstart for team-based token rate limiting, key management and user/team resource tracking.

**For additional details see the [Architecture Overview](./architecture.md)**

## Setup

`ADMIN_KEY` is a placeholder for the demo, see #wg-maas in Slack for the admin key. Use unique values for the `TEAM_ID`
and `USER_ID`, or use them to query existing usage etc. on that user and team.

All of these can be copied and pasted into the demo.

> Change the `TEAM_ID` value to try the demo, since it already exists.

```bash
export ADMIN_KEY="your-admin-key"
export TEAM_ID="test-team"
export USER_ID="test-user"
```

## Test Workflow

### 1. Create Team

```bash
curl -sk -X POST https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "'$TEAM_ID'",
    "team_name": "Test Team",
    "policy": "test-tokens",
    "token_limit": 100000,
    "time_window": "1h"
  }'
```

### 2. Create API Key

- Multiple keys or users can be added to a team. Users can have multiple keys in multiple teams.

```bash
API_RESPONSE=$(curl -sk -s -X POST https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/$TEAM_ID/keys \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "'$USER_ID'",
    "user_email": "'$USER_ID'@test.com"
  }')

API_KEY=$(echo "$API_RESPONSE" | grep -o '"api_key":"[^"]*"' | cut -d'"' -f4)
echo "API Key: $API_KEY"
```

### 3. Test Model Call

```bash
curl -s http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions \
  -H "Authorization: APIKEY $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-0-6b-instruct",
    "messages": [
      {
        "role": "user",
        "content": "Hello"
      }
    ],
    "max_tokens": 50
  }' | jq .
```

### 4. Update Team Configuration

Patch team limits, policy, or metadata (partial updates supported):

```bash
curl -sk -X PATCH https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/$TEAM_ID \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "token_limit": 20,
    "time_window": "1m"
  }'
```

### 5. Test Model Call with New Limits

```bash
curl -s http://qwen3-llm.apps.summit-gpu.octo-emerging.redhataicoe.com/v1/chat/completions \
  -H "Authorization: APIKEY $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-0-6b-instruct",
    "messages": [
      {
        "role": "user",
        "content": "Hello"
      }
    ],
    "max_tokens": 50
  }' | jq .
```

### 6. List Available Models

TODO: Enable user-scoped model access listing

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/models \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example Output

```json
{
  "models": [
    {
      "name": "qwen3-0-6b-instruct",
      "namespace": "llm",
      "url": "http://qwen3-0-6b-instruct-llm.apps.summit-gpu.octo-emerging.redhataicoe.com",
      "ready": true
    },
    {
      "name": "vllm-simulator",
      "namespace": "llm",
      "url": "http://vllm-simulator-llm.apps.summit-gpu.octo-emerging.redhataicoe.com",
      "ready": true
    }
  ]
}
```

### 7. List All Teams (Admin)

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example output:

```json
{
  "teams": [
    {
      "created_at": "2025-08-24T06:28:33Z",
      "description": "Default team for simple MaaS deployments - users without team assignment",
      "key_count": 13,
      "policy": "unlimited-policy",
      "team_id": "default",
      "team_name": "Default Team",
      "user_count": 9
    },
    {
      "created_at": "2025-08-24T08:06:23Z",
      "description": "Updated description for testing",
      "key_count": 1,
      "policy": "updated-policy",
      "team_id": "replicable-test",
      "team_name": "Updated Replicable Test Team",
      "user_count": 1
    },
    {
      "created_at": "2025-08-25T05:38:52Z",
      "description": "",
      "key_count": 7,
      "policy": "test-tokens",
      "team_id": "test-team",
      "team_name": "Test Team",
      "user_count": 2
    }
  ],
  "total_teams": 5
}
```

### 8. Get a Team Details (Admin)

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/$TEAM_ID \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example Output:

```json
{
  "created_at": "2025-08-25T06:17:35Z",
  "description": "",
  "key_count": 8,
  "keys": [
    "apikey-mittens-test-team-0119cba6",
    "apikey-mittens-test-team-ea36c31a",
    "apikey-testuser-test-team-107b866b",
    "apikey-testuser-test-team-3f7b1182",
    "apikey-testuser-test-team-4cdb4428",
    "apikey-testuser-test-team-7386fd4a",
    "apikey-testuser-test-team-805a7b80",
    "apikey-testuser-test-team-8c60ec98"
  ],
  "policy": "test-tokens",
  "team_id": "test-team",
  "team_name": "Test Team",
  "user_count": 2,
  "users": [
    {
      "user_id": "mittens",
      "user_email": "mittens@test.com",
      "role": "member",
      "team_id": "test-team",
      "team_name": "Test Team",
      "joined_at": "2025-08-25T05:39:04Z",
      "policy": "test-tokens"
    },
    {
      "user_id": "testuser",
      "user_email": "testuser@test.com",
      "role": "member",
      "team_id": "test-team",
      "team_name": "Test Team",
      "joined_at": "2025-08-24T23:08:48Z",
      "policy": "test-tokens"
    }
  ]
}
```

### 9. List Team Keys with Details (Admin)

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/$TEAM_ID/keys \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example Output

```json
{
  "keys": [
    {
      "created_at": "2025-08-25T05:39:04Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-mittens-test-team-0119cba6",
      "status": "active",
      "user_email": "mittens@test.com",
      "user_id": "mittens"
    },
    {
      "created_at": "2025-08-25T06:17:44Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-mittens-test-team-ea36c31a",
      "status": "active",
      "user_email": "mittens@test.com",
      "user_id": "mittens"
    },
    {
      "created_at": "2025-08-24T23:08:48Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-107b866b",
      "status": "active",
      "user_email": "testuser@test.com",
      "user_id": "testuser"
    },
    {
      "created_at": "2025-08-24T21:41:35Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-3f7b1182",
      "status": "active",
      "user_email": "testuser@test.com",
      "user_id": "testuser"
    },
    {
      "created_at": "2025-08-24T23:35:13Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-4cdb4428",
      "status": "active",
      "user_email": "testuser@test.com",
      "user_id": "testuser"
    },
    {
      "created_at": "2025-08-25T05:09:07Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-7386fd4a",
      "status": "active",
      "user_email": "testuser@test.com",
      "user_id": "testuser"
    },
    {
      "created_at": "2025-08-25T05:13:25Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-805a7b80",
      "status": "active",
      "user_email": "testuser@test.com",
      "user_id": "testuser"
    },
    {
      "created_at": "2025-08-25T05:02:38Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-8c60ec98",
      "status": "active",
      "user_email": "testuser@test.com",
      "user_id": "testuser"
    }
  ],
  "policy": "test-tokens",
  "team_id": "test-team",
  "team_name": "Test Team",
  "total_keys": 8,
  "total_users": 2,
  "users": [
    {
      "user_id": "mittens",
      "user_email": "mittens@test.com",
      "role": "member",
      "team_id": "test-team",
      "team_name": "Test Team",
      "joined_at": "2025-08-25T05:39:04Z",
      "policy": "test-tokens"
    },
    {
      "user_id": "testuser",
      "user_email": "testuser@test.com",
      "role": "member",
      "team_id": "test-team",
      "team_name": "Test Team",
      "joined_at": "2025-08-24T23:08:48Z",
      "policy": "test-tokens"
    }
  ]
}
```

### 10. List All User Keys Across Teams (Admin)

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/users/$USER_ID/keys \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example Output:

```json
{
  "keys": [
    {
      "created_at": "2025-08-24T23:08:48Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-107b866b",
      "status": "active",
      "team_id": "test-team",
      "team_name": "Test Team",
      "user_email": "testuser@test.com"
    },
    {
      "created_at": "2025-08-24T21:41:35Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-3f7b1182",
      "status": "active",
      "team_id": "test-team",
      "team_name": "Test Team",
      "user_email": "testuser@test.com"
    },
    {
      "created_at": "2025-08-24T23:35:13Z",
      "models_allowed": "",
      "policy": "test-tokens",
      "role": "member",
      "secret_name": "apikey-testuser-test-team-4cdb4428",
      "status": "active",
      "team_id": "test-team",
      "team_name": "Test Team",
      "user_email": "testuser@test.com"
    }
  ],
  "total_keys": 3,
  "user_id": "testuser"
}

```
### 11. Get User Usage Metrics (Admin)

- Example of a user with keys across various teams and usage for each of their keys.

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/users/$USER_ID/usage \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example output:

```json
{
  "user_id": "testuser",
  "total_token_usage": 1655,
  "total_authorized_calls": 69,
  "total_limited_calls": 8,
  "team_breakdown": [
    {
      "team_id": "debug-test",
      "team_name": "Debug Test Team",
      "policy": "debug-tokens",
      "token_usage": 19,
      "authorized_calls": 1,
      "limited_calls": 0
    },
    {
      "team_id": "ml-research-tokens",
      "team_name": "ml-research-tokens",
      "policy": "ml-research-tokens",
      "token_usage": 220,
      "authorized_calls": 10,
      "limited_calls": 0
    },
    {
      "team_id": "premium-tokens",
      "team_name": "premium-tokens",
      "policy": "premium-tokens",
      "token_usage": 354,
      "authorized_calls": 19,
      "limited_calls": 5
    },
    {
      "team_id": "test-team",
      "team_name": "Test Team",
      "policy": "test-tokens",
      "token_usage": 1062,
      "authorized_calls": 39,
      "limited_calls": 3
    }
  ],
  "last_updated": "2025-08-25T03:54:22.417070625Z"
}
```

### 12. Get Team Usage Metrics (Admin)

```bash
curl -sk -X GET https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/$TEAM_ID/usage \
  -H "Authorization: ADMIN $ADMIN_KEY" | jq .
```

- Example output (Total tokens followed by a user breakdown):

```json
{
  "team_id": "test-team",
  "team_name": "Test Team",
  "policy": "test-tokens",
  "total_token_usage": 1888,
  "total_authorized_calls": 69,
  "total_limited_calls": 7,
  "user_breakdown": [
    {
      "user_id": "mittens",
      "user_email": "mittens@test.com",
      "token_usage": 118,
      "authorized_calls": 5,
      "limited_calls": 1
    },
    {
      "user_id": "testuser",
      "user_email": "testuser@test.com",
      "token_usage": 1416,
      "authorized_calls": 47,
      "limited_calls": 4
    },
    {
      "user_id": "testuser2",
      "user_email": "testuser2@company.com",
      "token_usage": 354,
      "authorized_calls": 17,
      "limited_calls": 2
    }
  ],
  "last_updated": "2025-08-25T05:49:02.936449185Z"
}
```

### 13. Cleanup (Not Complete)

TODO: fix. Should deletes be cascading? e.g. If a team gets deleted, do the associated keys get removed to avoid
orphaned keys? A database should be considered eventually to make this cleaner or add some sort of reconciler.

```bash
curl -s -X DELETE https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/delete_key \
  -H "Authorization: ADMIN $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "key": "'$API_KEY'"
  }'

curl -sk -X DELETE https://key-manager-route-platform-services.apps.summit-gpu.octo-emerging.redhataicoe.com/teams/$TEAM_ID \
  -H "Authorization: ADMIN $ADMIN_KEY"
```

