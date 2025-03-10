# GitHub MCP Server

GitHub MCP Server implemented in Go.

## Setup

Create a GitHub Personal Access Token with the appropriate permissions
and set it as the GITHUB_PERSONAL_ACCESS_TOKEN environment variable.


## Tools

1. `get_me`
    - Return information about the authenticated user
2. `get_issue`
    - Get the contents of an issue within a repository.
    - Inputs
        - `owner` (string): Repository owner
        - `repo` (string): Repository name
        - `issue_number` (number): Issue number to retrieve
    - Returns: Github Issue object & details

## Standard input/output server

```sh
go run cmd/server/main.go stdio
```

E.g:

Set the PAT token in the environment variable and run:

```sh
script/get-me
```

And you should see the output of the GitHub MCP server responding with the user information.

```sh
GitHub MCP Server running on stdio
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"login\":\"juruen\",\"id\" ... }
      }
    ]
  }
}

```

