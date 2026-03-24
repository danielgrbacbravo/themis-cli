
# Themis API and Workflow Specification

## Purpose

This document is a language-agnostic specification for implementing a legitimate client against Themis, based on behavior evidenced in this repository.

It focuses on:

1. how authentication works
2. how session state should be managed
3. how to retrieve the course structure
4. how to retrieve assignment details
5. how to upload submissions
6. how to retrieve live grading updates

This is an integration spec, not an application architecture document.

## Scope

Observed primary origin:

- `https://themis.housing.rug.nl`

Observed authentication-related domains:

- `https://connect.surfconext.nl`
- `https://signon.rug.nl`
- `https://xfactor.rug.nl`

These domains are directly evidenced by the implemented login flow.

## Core Protocol Characteristics

Themis, as implemented here, behaves like:

- a cookie-authenticated web application
- with a structured JSON endpoint for course navigation
- with HTML-based pages for assignment metadata, submission history, and results
- with direct file endpoints for test-case files
- with a socket.io channel for live grading updates

This means a client must support:

- cookies
- redirects
- HTML form extraction
- HTML parsing
- multipart file upload
- optional socket.io subscriptions

## 1. Authentication Specification

## 1.1 Authentication Model

Observed authentication characteristics:

- login begins at a Themis endpoint
- authentication proceeds through an institutional SSO flow
- the flow includes username/password submission
- the flow includes TOTP-based MFA
- final authenticated state is represented by session cookies

Observed non-characteristics:

- no bearer token flow is evidenced
- no API key flow is evidenced
- no callback-URI-based OAuth helper is evidenced

## 1.2 Required Client Capabilities for Authentication

A client should support:

1. manual redirect inspection
2. cross-domain cookie continuity
3. HTML form discovery and replay
4. URL resolution for relative `Location` headers and relative form actions
5. form-urlencoded POST bodies
6. browser-like request headers during auth navigation

## 1.3 Login Flow

Suggested flow:

1. Start at `GET /log/in/oidc` on the Themis origin.
2. Read the redirect target from the `Location` header.
3. Follow the redirect chain until a non-redirect response is reached.
4. Parse the returned HTML for the first form.
5. Submit that form exactly as received.
6. Continue following redirects or form-based auto-submit pages.
7. Detect the credential form.
8. Submit username/password fields.
9. Continue following redirects or intermediate auto-submit forms.
10. Detect the MFA form by input name.
11. Submit the TOTP code.
12. Continue following redirects and form posts until returning to Themis.
13. Verify authenticated state by loading an authenticated Themis page.
14. Persist the resulting cookie jar.

## 1.4 Observed Login Endpoints and Request Forms

### Step A: Start Login

```http
GET https://themis.housing.rug.nl/log/in/oidc
```

Expected behavior:

- returns a redirect to the institutional login flow

## Step B: Follow SSO Redirects

The exact paths after `/log/in/oidc` are not fixed in this repository. They are server-provided via redirects and HTML forms.

A client should therefore:

- treat the SSO chain as dynamic
- not hardcode redirect destinations beyond the initial Themis entrypoint
- preserve cookies across all visited auth domains

## Step C: Submit Username and Password

Observed credential field names:

- `Ecom_User_ID`
- `Ecom_Password`
- `option=credential`

Suggested request form:

```http
POST <credential-form-action>
Content-Type: application/x-www-form-urlencoded
```

Body:

```text
...upstream hidden fields...
Ecom_User_ID=<username>
Ecom_Password=<password>
option=credential
```

## Step D: Submit MFA

Observed MFA field name:

- `nffc`

Suggested request form:

```http
POST <mfa-form-action>
Content-Type: application/x-www-form-urlencoded
```

Body:

```text
...upstream hidden fields...
nffc=<totp-code>
option=credential
```

## 1.5 Request Header Expectations During Login

The implemented client uses browser-like navigation headers during auth. A compatible client should strongly consider doing the same.

Observed header pattern:

```http
User-Agent: Mozilla/5.0 ...
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
Accept-Language: en-US,en;q=0.5
Accept-Encoding: gzip, deflate, br
Upgrade-Insecure-Requests: 1
Sec-Fetch-Dest: document
Sec-Fetch-Mode: navigate
Sec-Fetch-Site: none | same-origin | cross-site
Sec-Fetch-User: ?1
Connection: keep-alive
Cookie: <cookie string for target URL, if any>
Referer: <when applicable>
Origin: <when applicable for POST>
```

A client should preserve:

- correct cookies per target URL
- referer continuity
- origin on form posts when available

## 1.6 Detecting Successful Authentication

Observed verification strategy:

1. request `GET /course/`
2. parse returned HTML
3. look for the logged-in user anchor pointing to `/user`

Observed user pattern:

- anchor text containing a name and identifier like `s1234567` or `p1234567`

Suggested success rule:

- treat authentication as successful only after an authenticated Themis page is observed
- do not assume the SSO chain succeeded merely because redirects completed

## 1.7 Session Management

## Recommended Session State

A client should maintain:

- cookie jar as the primary authenticated state
- optional cached user identity extracted from Themis HTML
- optional session-valid flag derived from a successful authenticated page load

## Recommended Persistence

Persist:

- session cookies

Do not rely on persisting:

- one-time MFA codes

The repository persists credentials and the last entered TOTP, but that is an application choice, not a protocol requirement. Reusing stored TOTP later is unlikely to be reliable. That point is an inference from TOTP semantics.

## Recommended Session Reuse Flow

1. restore cookie jar
2. call `GET /course/`
3. if authenticated, continue
4. if not authenticated, perform fresh login

## Recommended Logout Flow

1. call `GET /log/out`
2. clear local cookie jar
3. clear persisted session state

## 2. Course Structure Specification

## 2.1 Primary Endpoint

Observed endpoint:

```http
GET /api/navigation{path}
```

Examples:

```http
GET /api/navigation/
GET /api/navigation/<course-path>
GET /api/navigation/<course-path>/<subpath>
```

## 2.2 Authentication Requirement

This endpoint should be treated as authenticated.

Attach:

- current Themis session cookies

## 2.3 Response Shape

Observed response type:

- JSON array

Observed item fields:

- `path`
- `title`
- `submitable`
- `isExam`
- `isQuiz`
- `visible`

Suggested internal model:

```text
NavigationNode
- path: string
- title: string
- isSubmittable: bool
- isExam: bool
- isQuiz: bool
- visible: bool
```

## 2.4 Suggested Retrieval Flow

1. authenticate first
2. request `GET /api/navigation/`
3. treat returned array as root nodes
4. for each non-leaf path, request `GET /api/navigation{path}` on demand
5. do not assume the full tree is returned in one response

## 2.5 Suggested State Management

Maintain:

- root node cache
- per-path child cache
- node metadata keyed by `path`

Suggested cache policy:

- lazy-load children
- keep path-indexed nodes in memory
- allow explicit refresh if stale data matters

## 3. Assignment Metadata Specification

## 3.1 Primary Endpoint

Observed endpoint:

```http
GET /course{assignmentPath}
```

This page serves several purposes:

- assignment metadata
- submission configuration
- test case file links
- grade or score display
- deadline display
- submission form discovery

## 3.2 Response Type

Observed response type:

- HTML

## 3.3 Observed HTML Structures

Assignment config:

- `.ass-config`
- `.cfg-group-title`
- `.cfg-line`
- `.cfg-key`
- `.cfg-val`

Test cases:

- `.subsec.round.shade`
- `h4.info` with text containing `Test`
- `.cfg-line a`

Grade-like values:

- `.grade`
- `.score`
- `.points`
- `.ass-grade`

Submission form:

- `form[action*='/submit/']`

## 3.4 Suggested Extraction Targets

A client should extract:

- grouped configuration values
- deadline or due date
- visible test case names
- input file URLs
- output file URLs
- submission form action
- submission language metadata

## 4. File Upload Specification

## 4.1 Upload Precondition

Do not upload directly from a guessed endpoint.

Suggested flow:

1. load the assignment page
2. parse the submission form
3. use the exact parsed form action for submission

This matters because the repository indicates the form action includes required query parameters such as:

- `_csrf`
- `sudo`

These are not generated client-side in the implementation. They are reused from the page.

## 4.2 Submission Endpoint

Observed form target pattern:

```text
/submit/<assignment-path>?_csrf=<redacted>&sudo=<redacted>
```

The exact path is discovered dynamically from the assignment page HTML.

## 4.3 Multipart Request Shape

Observed multipart fields:

- `files[]`
- `judgenow`
- `judgeLanguage`

Suggested request:

```http
POST <parsed-submit-form-action>
Content-Type: multipart/form-data; boundary=<generated>
Cookie: <session cookies>
```

Body parts:

```text
files[] = <file stream>
files[] = <file stream>
judgenow = true
judgeLanguage = <derived language>
```

## 4.4 Deriving `judgeLanguage`

Observed sources on the submission form:

- `data-languages`
- `data-suffixes`

Observed strategy:

1. read `data-suffixes` as a mapping from file suffix to language
2. read `data-languages` as default
3. inspect submitted file extensions
4. use the first matching language mapping
5. otherwise fall back to the default

## 4.5 Submission Success Detection

Observed success handling:

- `301/302`: use the `Location` header as the results path
- `200`: search returned HTML for a link to `/stats...`
- otherwise fall back to `/stats{assignmentPath}/@latest`

Observed error handling:

- if response status is error-like, parse:
  - `.error`
  - `.alert-danger`
  - `.alert`

## 4.6 Suggested Upload Flow

1. verify authenticated session
2. load assignment page
3. parse submission form and language metadata
4. build multipart request
5. submit files
6. capture results path
7. begin result retrieval or live watch

## 4.7 Suggested Upload State Management

Track:

- assignment path
- selected file set
- derived judge language
- current submission status
- results path returned by server

## 5. Live Grading Specification

## 5.1 Static Results Retrieval

Observed results paths:

- `/stats/...`
- `/submission/...`

Suggested retrieval:

```http
GET <results-path>
Cookie: <session cookies>
```

Response type:

- HTML

## 5.2 Results Parsing

Observed result structures:

- `section.submission`
- `text.percentage`
- `tr.sub-casetop`
- `td.sub-casename strong`
- `td.status-icon`
- `span[id^="show-hint-text-"]`
- `ul.sub-messages`

Observed statuses:

- `passed`
- `failed`
- `error`
- `pending`
- `queued`
- `none`
- `compilerr`
- `runtimerr`
- `timeout`
- `outlimit`
- `diff`
- `deadline`

Suggested internal result model:

```text
SubmissionResult
- overallStatus
- percentage
- summary
- testCases[]

TestCaseResult
- name
- status
- message
- grade
- hint
```

## 5.3 Live Results Channel

Observed live namespace:

```text
socket.io namespace: /submission
```

Observed connection behavior:

- send session cookies with socket handshake
- use transports:
  - websocket
  - polling

Observed watch event:

```text
event: watch
payload: { path: "<results-path>", op: "replace", open: true }
```

Observed unwatch event:

```text
event: unwatch
payload: { path: "<results-path>" }
```

Observed update event:

```text
event: submission
payload contains: html
```

Observed update handling:

- parse `payload.html` with the same results parser used for page loads

## 5.4 Suggested Live Grading Flow

1. obtain a results path after submission
2. immediately fetch the current results page
3. connect to the `/submission` socket namespace
4. send `watch` for that path
5. on each `submission` event, parse returned HTML
6. stop watching when a terminal result is reached

## 5.5 Suggested Polling Fallback

Observed implementation fallback:

- poll every few seconds while no live terminal state is reached

Suggested fallback strategy:

1. if socket connection fails, poll the results path
2. continue polling while overall status is non-terminal
3. stop polling when status becomes terminal

Suggested terminal statuses:

- `passed`
- `failed`
- `error`
- `compilerr`
- `runtimerr`
- `timeout`
- `outlimit`
- `diff`
- `deadline`

## 5.6 Suggested Live State Management

Track:

- results path
- last known overall status
- latest parsed result object
- whether live subscription is active
- whether polling fallback is active

## 6. Raw Test File Retrieval

## 6.1 File Endpoint Behavior

Observed file handling:

1. file URLs may be relative or absolute
2. if the path starts with `/file/`, append `raw=true` when missing
3. request the file as plain text with session cookies

Suggested request:

```http
GET /file/<path>?raw=true
Cookie: <session cookies>
```

## 6.2 Suggested Flow

1. load assignment page
2. parse test-case links
3. normalize each file URL to include `raw=true`
4. fetch input/output files as text

## 7. Suggested Client State Model

This section is workflow-oriented, not architecture-specific.

Maintain these state domains:

### Authentication State

- cookie jar
- authenticated user identity, if known
- session validity timestamp or last-check timestamp

### Navigation State

- root course nodes
- children indexed by parent path
- optional refresh markers

### Assignment State

- assignment metadata keyed by assignment path
- parsed submission form action
- parsed language metadata
- parsed test case file links

### Submission State

- active assignment path
- selected files
- derived judge language
- last submission attempt result
- latest results path

### Results State

- current results path
- latest parsed submission result
- live subscription status
- polling fallback status

## 8. Suggested End-to-End Workflows

## 8.1 Authenticate

1. attempt session reuse via cookies
2. if session invalid, run full login flow
3. verify by loading an authenticated Themis page
4. persist cookies

## 8.2 Load Course Structure

1. ensure session is valid
2. fetch `/api/navigation/`
3. lazily fetch child paths using `/api/navigation{path}`

## 8.3 Load Assignment

1. ensure session is valid
2. fetch `/course{assignmentPath}`
3. parse metadata, test cases, and submission form

## 8.4 Submit

1. ensure session is valid
2. reload or confirm assignment page
3. parse submit action and language metadata
4. upload multipart form
5. capture results path

## 8.5 Watch Grading

1. fetch current results immediately
2. subscribe to live updates
3. fall back to polling if needed
4. stop when terminal state is reached

## 9. Compatibility Notes

Directly evidenced stable-looking contracts:

- `GET /api/navigation{path}` returning JSON
- `GET /course{path}` for assignment pages
- `GET /stats{path}` for history pages
- results paths under `/stats/...` or `/submission/...`
- raw file access via `/file/...?...raw=true`
- live updates over socket.io namespace `/submission`

Potentially fragile contracts:

- HTML selectors used for auth forms
- HTML selectors used for assignment parsing
- HTML selectors used for results parsing
- exact contents of live `submission` event payloads beyond the observed `html` field

## 10. Proven Facts vs Inferences

Proven from repository behavior:

- authenticated state is cookie-based
- login starts at `/log/in/oidc`
- username/password fields are `Ecom_User_ID` and `Ecom_Password`
- MFA field is `nffc`
- course navigation uses `/api/navigation{path}`
- uploads use a parsed `/submit/...` form action
- live grading uses socket.io namespace `/submission`
- static grading pages are parsed from HTML

Inferences:

- the upstream platform likely expects browser-like navigation semantics during auth, because the client explicitly imitates them
- session cookie reuse is the preferred long-lived auth strategy
- persisting TOTP codes is not a robust long-term session renewal strategy

## 11. Implementation Checklist

For a new client, the minimum viable sequence is:

1. implement cookie jar persistence
2. implement browser-like GET/POST helpers
3. implement redirect following
4. implement HTML form extraction
5. implement login flow
6. implement authenticated session verification via `/course/`
7. implement `/api/navigation{path}`
8. implement assignment-page parsing
9. implement multipart upload using parsed form action
10. implement results-page parsing
11. optionally implement live socket watch and polling fallback
