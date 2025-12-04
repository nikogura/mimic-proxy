# Aiprise KYC Integration in Terrace Platform

## Overview

The Terrace platform uses **Aiprise** as its identity verification provider for KYC (Know Your Customer) compliance. The integration spans three repositories:

1. **dashboard-service** - Frontend (Next.js/React) using Aiprise Web SDK v2.1.1
2. **id-verification-service** - Dedicated backend microservice handling Aiprise callbacks
3. **api-service** - Main backend API that enforces KYC requirements for trading

The system operates in both sandbox and production Aiprise environments with end-to-end webhook-based verification flow.

## Data Flow Architecture

### Requests to Aiprise (Frontend → Aiprise)

**Initialization Data Sent:**
- `mode`: "SANDBOX" or "PRODUCTION"
- `template-id`: "cfd68601-7398-4ecd-bdab-50c745aa807b"
- `callback-url`: `https://api.terrace.fi/id-verification/v1/aiprise-callback` (production)
- `events-callback-url`: Same as callback-url
- `client-reference-id`: User's UUID
- `client-reference-data`: `{ username: string }`
- `user-data`: `{ first_name, last_name, email_address }`
- `theme`: Custom Terrace branding (chartreuseGreen, black, Inter font)

### Responses from Aiprise (Aiprise → Frontend)

**Event Structure:**
```typescript
{
  verification_session_id: string;
  template_id: string;
  status: "STARTED" | "SUCCESSFUL" | "ERROR";
  status_text?: string; // Error description for ERROR status
  timestamp: string; // RFC3339 format
}
```

**Four Event Types:**
1. `aiprise:started` - Verification session initiated
2. `aiprise:successful` - User completed verification
3. `aiprise:error` - Errors: SESSION_FAILED, SESSION_EXPIRED, SESSION_COMPLETED
4. `aiprise:continue` - Custom continue flow trigger

### Backend Communication (Frontend → Terrace API)

**GraphQL Mutation to `/id-verification/v1/event`:**
```typescript
{
  verification_session_id: string;
  template_id: string;
  status: "STARTED" | "ABANDONED" | "SUCCESSFUL" | "ERROR";
  status_text?: string;
  timestamp: string; // RFC3339 format
}
```

## Status Mappings

**Aiprise → Database:**
- STARTED → STARTED
- SUCCESSFUL → SUBMITTED (awaiting backend review)
- ERROR → ERROR

**Database → User-Facing:**
- STARTED/ABANDONED/AIPRISE-STARTED → NONE (not verified)
- TRIGGERED/ERROR/SUBMITTED/IN_REVIEW/UNKNOWN → IN_REVIEW (pending)
- APPROVED → VERIFIED (access granted)
- DECLINED → REJECTED (must retry)
- FAILED_AML → FAILED_AML (AML check failed)

## Key Implementation Details

**Files:**
- `component/new/kyc/aiprise-frame.tsx:1` - Core SDK integration
- `toolkit/kyc.ts:1` - Status logic & GraphQL mutations
- `stores/AccountStore.ts:1` - State management with 5-second polling when IN_REVIEW
- `next.config.js:1` - CSP allows Aiprise iframe domains

**Environments:**
- **Sandbox**: api-sandbox.aiprise.com, verify-sandbox.aiprise.com
- **Production**: api.aiprise.com, verify.aiprise.com

**Access Control:**
- Feature flag: `ENABLE_KYC_FOR` (ALL | EU_AND_TEAMS | disabled)
- Trading UI blocks non-verified users from submitting orders
- Settings page shows verification status with retry option

## Complete Data Exchange Summary

### 1. Aiprise Package Dependency

**File:** `package.json`

```json
"aiprise-web-sdk": "^2.1.1"
```

### 2. Configuration Files

#### Environment Variables Definition
**File:** `types/env.d.ts`

```typescript
NEXT_PUBLIC_AIPRISE_API_URL: string;
NEXT_PUBLIC_AIPRISE_ENVIRONMENT: "SANDBOX" | "PRODUCTION";
NEXT_PUBLIC_AIPRISE_IFRAME_URL: string;
NEXT_PUBLIC_AIPRISE_TEMPLATE_ID: string;
```

#### Environment Configurations by Deployment Stage

**Development (Sandbox)** - `env-files/.env-dev-01`:
```
NEXT_PUBLIC_AIPRISE_API_URL=https://api-sandbox.aiprise.com
NEXT_PUBLIC_AIPRISE_ENVIRONMENT=SANDBOX
NEXT_PUBLIC_AIPRISE_IFRAME_URL=https://verify-sandbox.aiprise.com
NEXT_PUBLIC_AIPRISE_TEMPLATE_ID=cfd68601-7398-4ecd-bdab-50c745aa807b
NEXT_PUBLIC_API_SERVICE_ENDPOINT=https://api-service-dev-01.terrace.fi
```

**Staging (Sandbox)** - `env-files/.env-stage-01`:
```
NEXT_PUBLIC_AIPRISE_API_URL=https://api-sandbox.aiprise.com
NEXT_PUBLIC_AIPRISE_ENVIRONMENT=SANDBOX
NEXT_PUBLIC_AIPRISE_IFRAME_URL=https://verify-sandbox.aiprise.com
NEXT_PUBLIC_AIPRISE_TEMPLATE_ID=cfd68601-7398-4ecd-bdab-50c745aa807b
NEXT_PUBLIC_API_SERVICE_ENDPOINT=https://api-service-stage-01.terrace.fi
```

**Production (Live)** - `env-files/.env-prod-01`:
```
NEXT_PUBLIC_AIPRISE_API_URL=https://api.aiprise.com
NEXT_PUBLIC_AIPRISE_ENVIRONMENT=PRODUCTION
NEXT_PUBLIC_AIPRISE_IFRAME_URL=https://verify.aiprise.com
NEXT_PUBLIC_AIPRISE_TEMPLATE_ID=cfd68601-7398-4ecd-bdab-50c745aa807b
NEXT_PUBLIC_API_SERVICE_ENDPOINT=https://api.terrace.fi
```

#### Content Security Policy (CSP) Configuration
**File:** `next.config.js`

The CSP headers allow Aiprise iframe:
```javascript
frame-src 'self' blob: https://turnkey.io blob: https://export.turnkey.com blob: https://auth.turnkey.com blob: https://import.turnkey.com blob: ${aipriseIframeUrl} ${onRamperIframeUrl} ...

connect-src ... ${aipriseApiUrl} ...
```

### 3. Aiprise Frame Component (Web SDK Integration)

**File:** `component/new/kyc/aiprise-frame.tsx`

This is the core Aiprise integration component that:

**Imports the SDK:**
```typescript
import "aiprise-web-sdk";
```

**Renders the Aiprise Frame Component:**
```tsx
<aiprise-frame
  ref={verifyFrameRef}
  mode={process.env.NEXT_PUBLIC_AIPRISE_ENVIRONMENT}
  template-id={process.env.NEXT_PUBLIC_AIPRISE_TEMPLATE_ID!}
  callback-url={`${process.env.NEXT_PUBLIC_API_SERVICE_ENDPOINT}/id-verification/v1/aiprise-callback`}
  events-callback-url={`${process.env.NEXT_PUBLIC_API_SERVICE_ENDPOINT}/id-verification/v1/aiprise-callback`}
  client-reference-id={currentUser.id}
  client-reference-data={JSON.stringify({
    username: currentUser.username,
  })}
  user-data={JSON.stringify({
    first_name: currentUser.first_name,
    last_name: currentUser.last_name,
    email_address: currentUser.email,
  })}
  theme={JSON.stringify({
    color_page: "#202020",
    color_brand: chartreuseGreen,
    color_brand_overlay: black,
    font_name: "Inter",
  })}
  allow="camera; microphone; fullscreen;"
/>
```

**Event Listeners:**

The component listens for four Aiprise events:

1. **`aiprise:started`** - Fired when verification session starts
   - Sends "STARTED" status to backend via GraphQL mutation
   - Captures `verification_session_id` from event

2. **`aiprise:successful`** - Fired when user completes verification
   - Sends "SUCCESSFUL" status to backend
   - Shows success toast notification

3. **`aiprise:error`** - Fired on verification errors
   - Handles three error codes:
     - `SESSION_FAILED` - Session creation failed
     - `SESSION_EXPIRED` - Session has expired
     - `SESSION_COMPLETED` - Session already completed by user
   - Sends "ERROR" status with error code to backend

4. **`aiprise:continue`** - Fired for custom continue flow
   - Calls `onContinue()` callback

**Event Callback Structure:**
```typescript
{
  verification_session_id: string;
  template_id: string;
  status: "STARTED" | "SUCCESSFUL" | "ERROR";
  status_text?: string;
  timestamp: string; // RFC3339 format
}
```

### 4. KYC Modal Wrapper Component

**File:** `component/new/kyc/kyc-modal.tsx`

Wraps the AipriseFrame with a two-step modal:

- **Step 1 (intro):** Shows "Verify your identity" intro screen with ~2 minute estimation
- **Step 2 (frame):** Displays the actual Aiprise iframe

Refreshes account store on successful verification completion.

### 5. KYC Utility Functions

**File:** `toolkit/kyc.ts`

**Key Functions:**

1. **`sendKycCallbackEvent()`** - Sends verification status updates to backend
   - Uses GraphQL mutation: `SEND_KYC_CALLBACK_EVENT`
   - Endpoint: `/id-verification/v1/event`

2. **`useEnableKyc()`** - Determines if KYC is enabled for user
   - Based on feature flag: `ENABLE_KYC_FOR` (values: "EU_AND_TEAMS", "ALL", disabled)
   - Can be forced via `enable-kyc` cookie

3. **`useRequiresKYC()`** - Returns true if user must complete KYC
   - Checks if KYC is enabled AND `kycStatus != "VERIFIED"`

4. **Status Mapping:**
   - `IdVerificationSessionStatus` includes: STARTED, ABANDONED, TRIGGERED, ERROR, AIPRISE-STARTED, SUBMITTED, APPROVED, DECLINED, IN_REVIEW, UNKNOWN, FAILED_AML
   - Maps to `KycStatus`: NONE, IN_REVIEW, VERIFIED, REJECTED, FAILED_AML

**Status Transformation Logic:**
```typescript
const IdVerificationSessionStatusToKycStatus = {
  STARTED: "NONE",
  ABANDONED: "NONE",
  TRIGGERED: "IN_REVIEW",
  ERROR: "IN_REVIEW",
  "AIPRISE-STARTED": "NONE",
  SUBMITTED: "IN_REVIEW",
  APPROVED: "VERIFIED",
  DECLINED: "REJECTED",
  IN_REVIEW: "IN_REVIEW",
  UNKNOWN: "IN_REVIEW",
  FAILED_AML: "FAILED_AML",
};
```

### 6. GraphQL Mutation for KYC Callbacks

**File:** `rest/sendKycCallbackEvent.ts`

```graphql
mutation SendKycCallbackEvent($input: SendKycEventVariables!) {
  sendKycCallbackEvent(input: $input)
    @rest(
      type: "SendKycEvent"
      path: "/id-verification/v1/event"
      method: "POST"
    ) {
    message
  }
}
```

**Input Variables:**
```typescript
{
  verification_session_id: string;
  template_id: string;
  status: "STARTED" | "ABANDONED" | "SUCCESSFUL" | "ERROR";
  status_text?: string;
  timestamp: string; // RFC3339 format
}
```

### 7. KYC Status Banners & UI Components

**File:** `component/new/kyc/kyc-verification-banners.tsx`

Shows contextual banners based on KYC status:

1. **`VERIFIED` (green)** - "Your identity verification is verified. You now have access to all venues."
   - Shown only if verified within last 24 hours
   - Can be dismissed

2. **`REJECTED` (red)** - "There was an issue with your identify verification. Please try again."
   - Includes "Verify Now" button to retry

3. **`IN_REVIEW` (default)** - "Your identity verification is under review. Once verified, you will be granted access to all venues."

4. **`NONE` (green gradient)** - "Verify your identity to unlock full trading and features."
   - Shown when no verification started

**Settings Component:**
**File:** `component/new/kyc/settings-identity-verification.tsx`

Displays KYC status in account settings with ability to start verification.

### 8. Global Modal Integration

**File:** `component/new/dashboard/global-modals.tsx`

- KycModal is dynamically loaded (SSR disabled)
- Can be triggered via `openKycModal()` function through EventStore
- Integrated into global modals system alongside deposit, withdraw, and authentication modals

### 9. Account Store KYC State Management

**File:** `stores/AccountStore.ts`

**Data Structure:**
```typescript
type PickedCurrentUser = Web_Auth_GetCurrentSessionQuery["terrace_schema_users"][0];

// Contains arrays:
approved_id_verification_sessions: Array<{
  user_id: uuid;
  status: string;
  verification_session_id: uuid;
  created_at: timestamp;
  updated_at: timestamp;
  first_callback_at?: timestamp;
}>;

id_verification_sessions: Array<{
  user_id: uuid;
  status: string;
  verification_session_id: uuid;
  created_at: timestamp;
  updated_at: timestamp;
  first_callback_at?: timestamp;
}>;
```

**Transformer Function:**
```typescript
const transformer = (value: InitialAccountStore): AccountStore => {
  return {
    ...value,
    idVerificationSession:
      value.currentUser?.id_verification_sessions?.[0] ?? null,
    kycStatus: getKycStatusFromVerificationSessions({
      approvedSession: value.currentUser?.id_verification_sessions?.[0],
      latestSession: value.currentUser?.id_verification_sessions?.[0],
    }),
    // ... other fields
  };
};
```

**Auto-Refresh Logic:**
When `kycStatus === "IN_REVIEW"`, account store auto-refreshes every 5 seconds to detect completion.

### 10. GraphQL Query for Verification Sessions

**File:** `toolkit/auth.ts`

Fetches verification session data through Hasura:
```graphql
approved_id_verification_sessions: id_verification_sessions(
  where: { status: { _eq: "APPROVED" } }
  limit: 1
)

id_verification_sessions(
  where: { status: { _nin: ["STARTED", "ABANDONED", "AIPRISE-STARTED"] } }
  order_by: { created_at: desc, updated_at: desc }
  limit: 1
)
```

### 11. KYC Enforcement in Trading UI

Multiple components check KYC status before allowing trading:

- `component/spot/v3/SpotOrderForms.tsx`
- `component/spot/KycRequiredToolTip.tsx`
- `component/spot/PathFinderSubmitButton.tsx`

### 12. Feature Flag Control

**File:** `lib/feature-flags.ts`

KYC can be controlled via feature flag: `ENABLE_KYC_FOR`

Values:
- `"ALL"` - Require KYC for all users
- `"EU_AND_TEAMS"` - Require KYC only for EU users or team accounts
- Not set/disabled - KYC optional

## Complete KYC Flow

1. User navigates to trading page or clicks "Verify Now"
2. `KycModal` is opened, showing intro screen
3. User clicks "Get started"
4. `AipriseFrame` web component loads with:
   - User ID as `client-reference-id`
   - User's first name, last name, email as pre-filled data
   - Terrace theming
   - Custom callback URLs pointing to backend verification endpoint
5. User completes identity verification in Aiprise iframe
6. Aiprise emits events (started, successful, error)
7. Frontend captures events and sends status via GraphQL mutation to `/id-verification/v1/event`
8. Backend updates `id_verification_sessions` table in Hasura
9. Account store polls backend every 5 seconds (while IN_REVIEW)
10. Once approved, banner shows success, user gains full trading access

## Key Files Summary

| File | Purpose |
|------|---------|
| `component/new/kyc/aiprise-frame.tsx` | Core Aiprise SDK integration & event handling |
| `component/new/kyc/kyc-modal.tsx` | Modal wrapper for KYC flow |
| `toolkit/kyc.ts` | KYC utilities & status mapping |
| `rest/sendKycCallbackEvent.ts` | GraphQL mutation for callbacks |
| `stores/AccountStore.ts` | KYC state management |
| `component/new/kyc/kyc-verification-banners.tsx` | Status banners UI |
| `component/new/kyc/settings-identity-verification.tsx` | Settings page integration |
| `next.config.js` | CSP configuration for Aiprise |
| `types/env.d.ts` | Aiprise environment variables |
| `env-files/.env-*` | Environment configurations |

## Integration Summary

The integration is iframe-based, frontend-initiated, with status events proxied through the Terrace backend for verification and storage. All user data flows through secure HTTPS connections with proper CSP headers, and the verification state is managed through a combination of frontend state management and backend database records queried via GraphQL.

---

# Backend Services: id-verification-service & api-service

## Architecture Overview

The KYC verification backend consists of two services:

1. **id-verification-service** - Dedicated microservice that:
   - Receives webhooks from Aiprise
   - Processes verification lifecycle events
   - Manages database records via Hasura GraphQL
   - Deactivates users in FusionAuth on AML failures
   - Exposes Prometheus metrics

2. **api-service** - Main backend API that:
   - Enforces KYC requirements for trading
   - Queries verification status from Hasura
   - Caches APPROVED status with 24-hour TTL
   - Blocks orders on regulated venues without KYC approval

---

## id-verification-service

**Repository:** `~/project/github.com/subdialia/id-verification-service/`

### REST API Endpoints

**File:** `pkg/rest/server.go:55-56`

```
POST /v1/event               - Frontend events (JWT authenticated)
POST /v1/aiprise-callback    - Aiprise callbacks (HMAC authenticated)
```

### 1. Aiprise API Client

**File:** `pkg/aiprise/client.go:1-89`

**Purpose:** Queries Aiprise API for verification results

**Endpoint:** `GET {AIPRISE_URL}/api/v1/verify/get_user_verification_result/{verification_session_id}`

**Authentication:** `X-API-KEY` header

**Request Structure:**
```go
type Client interface {
    GetUserVerificationResult(verificationSessionId string) (*UserVerificationResult, error)
}
```

**Response Structure:**
```go
type UserVerificationResult struct {
    AiPriseSummary        *AiPriseSummary `json:"aiprise_summary"`
    Status                string          `json:"status"` // NOT_STARTED|RUNNING|PENDING|FAILED|COMPLETED
    VerificationSessionId string          `json:"verification_session_id"`
    ClientReferenceId     *string         `json:"client_reference_id"`
    AmlInfo               *AmlInfo        `json:"aml_info"`
}

type AiPriseSummary struct {
    VerificationResult string `json:"verification_result"` // APPROVED|DECLINED|REVIEW|UNKNOWN
}

type AmlInfo struct {
    EntityHits []EntityHit `json:"entity_hits"`
}

type EntityHit struct {
    EntityType     *string  `json:"entity_type"` // PERSON|COMPANY|ORGANISATION|UNKNOWN
    Name           *string  `json:"name"`
    NameMatchScore *float64 `json:"name_match_score"`
    AlsoKnownAs    []string `json:"also_known_as"`
    AmlHits        []AmlHit `json:"aml_hits"`
}

type AmlHit struct {
    HitType *string `json:"hit_type"` // PEP|SANCTION|ADVERSE_MEDIA|WARNING|FITNESS_PROBITY|CRIMINAL_RECORD|LEGAL_BACKGROUND|UNKNOWN
}
```

### 2. Aiprise Callback Handler

**File:** `pkg/rest/aiprise_callback_handler.go:1-149`

**Authentication:** HMAC-SHA256 signature verification via `X-HMAC-SIGNATURE` header (lines 31-54)

**Request Body:**
```go
type AiPriseCallbackHandlerCommonRequestBody struct {
    EventType             *string `json:"event_type"`
    VerificationSessionId *string `json:"verification_session_id"`
    Data                  *struct {
        VerificationSessionId *string `json:"verification_session_id"`
    } `json:"data"`
}
```

**Supported Event Types (lines 99-114):**
1. `INITIAL_CALLBACK` - Final verification decision
2. `VERIFICATION_SESSION_STARTED` - Session initiated by Aiprise
3. `VERIFICATION_REQUEST_SUBMITTED` - User submitted documents
4. `CASE_STATUS_UPDATE` - Manual status change by Aiprise admin
5. `AML_MONITORING_UPDATE` - Ongoing AML monitoring detected changes

### 3. Event Handlers

#### Initial Callback Handler
**File:** `pkg/rest/aiprise_callback_handle_initial_callback.go:1-145`

**Purpose:** Processes final verification decision from Aiprise

**Request Body:**
```go
type AiPriseCallbackHandlerInitialCallbackRequestBody struct {
    AiPriseSummary        *AiPriseSummary `json:"aiprise_summary"`
    Status                string          `json:"status"`
    VerificationSessionId string          `json:"verification_session_id"`
    ClientReferenceId     *string         `json:"client_reference_id"`
    AmlInfo               *AmlInfo        `json:"aml_info"`
}
```

**Processing Logic:**
1. If `AmlInfo` present with entity hits → Status: `FAILED_AML`, deactivate user in FusionAuth
2. If `APPROVED` → Status: `APPROVED`
3. If `DECLINED` → Status: `DECLINED`
4. If `REVIEW` → Status: `IN_REVIEW`
5. If `UNKNOWN` → Status: `UNKNOWN`

#### Case Status Update Handler
**File:** `pkg/rest/aiprise_callback_handle_case_status_update.go:1-97`

**Purpose:** Handles manual status changes by Aiprise admins

**Status Mapping (lines 47-64):**
- Aiprise `APPROVED` → Database `APPROVED`
- Aiprise `DECLINED` → Database `DECLINED`
- Aiprise `REVIEW` → Database `IN_REVIEW`
- Aiprise `UNKNOWN` → Database `UNKNOWN`

#### AML Monitoring Handler
**File:** `pkg/rest/aiprise_callback_handle_aml_monitoring_update.go:1-156`

**Purpose:** Processes ongoing AML monitoring updates

**Request Body:**
```go
type AiPriseCallbackHandlerAmlMonitoringUpdateRequestBody struct {
    EventType string `json:"event_type"`
    Data      struct {
        VerificationSessionId string `json:"verification_session_id"`
        AmlMonitoringUpdate   struct {
            New     *[]EntityHit `json:"new"`
            Removed *[]EntityHit `json:"removed"`
            Updated *[]EntityHit `json:"updated"`
        } `json:"aml_monitoring_update"`
    } `json:"data"`
}
```

**Processing Logic:**
- If new or updated AML hits → Status: `FAILED_AML`, deactivate user
- If only removed hits → Log, no action

#### Session Started Handler
**File:** `pkg/rest/aiprise_callback_handle_started.go:1-73`

**Purpose:** Records when Aiprise verification session starts

#### Submitted Handler
**File:** `pkg/rest/aiprise_callback_handle_submitted.go:1-73`

**Purpose:** Records when user submits verification documents

### 4. Frontend Event Handler

**File:** `pkg/rest/event_handler.go:1-269`

**Purpose:** Processes events from dashboard-service frontend

**Authentication:** JWT Bearer token validation via FusionAuth (lines 30-53)

**Request Body (lines 20-26):**
```go
type EventHandlerRequestBody struct {
    VerificationSessionId string  `json:"verification_session_id"` // required
    TemplateId            string  `json:"template_id"`              // required
    Status                string  `json:"status"`                   // required: STARTED|ABANDONED|SUCCESSFUL|ERROR
    StatusText            *string `json:"status_text"`
    Timestamp             string  `json:"timestamp"`                // required, RFC3339
}
```

**Status Mappings (lines 255-267):**
- `STARTED` → `STARTED`
- `ABANDONED` → `ABANDONED`
- `SUCCESSFUL` → `TRIGGERED`
- `ERROR` → `ERROR`

### 5. Database Schema

**Table:** `terrace_schema.id_verification_sessions`

**Fields:**
```sql
verification_session_id UUID PRIMARY KEY
user_id                 UUID NOT NULL
template_id             UUID NOT NULL
status                  VARCHAR NOT NULL
status_text             TEXT
created_at              TIMESTAMP NOT NULL
updated_at              TIMESTAMP NOT NULL
triggered_at            TIMESTAMP          -- When frontend triggered
first_callback_at       TIMESTAMP          -- When Aiprise first responded
```

**Status Constants (pkg/rest/toolkit.go:36-48):**
```go
const (
    IdVerificationSessionStatusStarted          = "STARTED"
    IdVerificationSessionStatusAbandoned        = "ABANDONED"
    IdVerificationSessionStatusFeTriggered      = "TRIGGERED"
    IdVerificationSessionStatusFeError          = "ERROR"
    IdVerificationSessionStatusAipriseStarted   = "AIPRISE-STARTED"
    IdVerificationSessionStatusAiPriseSubmitted = "SUBMITTED"
    IdVerificationSessionStatusApproved         = "APPROVED"
    IdVerificationSessionStatusDeclined         = "DECLINED"
    IdVerificationSessionStatusInReview         = "IN_REVIEW"
    IdVerificationSessionStatusBeUnknown        = "UNKNOWN"
    IdVerificationSessionStatusFailedAml        = "FAILED_AML"
)
```

### 6. Status State Machine

**File:** `pkg/rest/toolkit.go:79-173`

**Purpose:** Validates allowed status transitions

**Key Rules:**
- `FAILED_AML` is a terminal state (no transitions allowed)
- `APPROVED`, `DECLINED`, `IN_REVIEW` can only progress to final states
- `STARTED` can transition to most other states
- State transitions are strictly validated before database updates

### 7. GraphQL Manager

**File:** `pkg/gql/manager.go`

**Purpose:** Interface for database operations via Hasura GraphQL

```go
type Manager interface {
    GetIdVerificationSession(ctx context.Context, verificationSessionId uuid.UUID) (*GetIdVerificationSessionResponse, error)
    GetLatestIdVerificationSessionForUser(ctx context.Context, userId uuid.UUID) (*GetLatestIdVerificationSessionForUserResponse, error)
    GetIdVerificationSessionsForUser(ctx context.Context, userId uuid.UUID, offset int, limit int) (*GetIdVerificationSessionsForUserResponse, error)
    InsertIdVerificationSession(ctx context.Context, idVerificationSession IdVerificationSession) (*InsertIdVerificationSessionResponse, error)
    UpdateIdVerificationSession(ctx context.Context, verificationSessionId uuid.UUID, updates map[string]interface{}) (*UpdateIdVerificationSessionResponse, error)
}
```

### 8. FusionAuth Integration

**File:** `pkg/fusionauth/client.go:1-42`

**Purpose:** User management in FusionAuth identity provider

```go
type Client interface {
    ValidateJWT(encodedJWT string) (*fusionauth.ValidateResponse, error)
    DeactivateUser(userId string) (*fusionauth.BaseHTTPResponse, *fusionauth.Errors, error)
}
```

**Usage:** When AML check fails, service immediately deactivates user to prevent re-login

### 9. CLI Commands

#### Status Command
**Usage:** `./id-verification-service status <user_id>`

**Purpose:** Lists all verification sessions for user
- Paginated in 1000-record batches
- Shows latest status first
- Displays table with: ID, Status, Status Text, Created/Updated/Triggered/First Callback timestamps

#### Refresh Command
**Usage:** `./id-verification-service refresh <user_id>`

**Purpose:** Manually reconciles verification status
- Fetches latest verification session
- Queries current Aiprise status via API
- Updates database if status changed
- Handles AML deactivation if needed

**File:** `pkg/refresh/refresh.go:1-120`

### 10. Prometheus Metrics

**File:** `pkg/metrics/counters.go`

**Key Metrics:**

**Event Counters:**
- `id_verification_service_event_started` - Frontend STARTED events
- `id_verification_service_event_abandoned` - Frontend ABANDONED events
- `id_verification_service_event_successful` - Frontend SUCCESSFUL events
- `id_verification_service_event_error` - Frontend ERROR events

**Callback Counters:**
- `id_verification_service_callback_initial_callback_*` - By result (APPROVED, DECLINED, REVIEW, UNKNOWN)
- `id_verification_service_callback_case_status_update_*` - By status
- `id_verification_service_callback_aml_monitoring_update` - AML updates
- `id_verification_service_callback_auth_success` - HMAC auth successes
- `id_verification_service_callback_auth_failure` - HMAC auth failures

**AML Counters:**
- `id_verification_service_aml_failure_initial_callback` - AML failures from initial callback
- `id_verification_service_aml_failure_monitoring_update` - AML failures from monitoring

**User Deactivation:**
- `id_verification_service_user_deactivation_failed` - Failed FusionAuth deactivations (alerts engineering)

**Latency Histogram:**
- `id_verification_service_aiprise_latency_s` - Time from user triggering frontend to Aiprise callback

### 11. Configuration

**Required Environment Variables:**
```bash
AIPRISE_URL=https://api-sandbox.aiprise.com  # or https://api.aiprise.com for production
AIPRISE_API_KEY=<secret>
AIPRISE_HMAC_SECRET=<secret>
HASURA_ENDPOINT=https://hasura-dev-01.corp.terrace.fi/v1/graphql
HASURA_ADMIN_SECRET=<secret>
HASURA_ROLE=admin
FUSIONAUTH_URL=https://terrace-dev.fusionauth.io
FUSIONAUTH_API_KEY=<secret>
REST_ADDRESS=0.0.0.0:9999
METRICS_ADDRESS=0.0.0.0:8080
```

---

## api-service KYC Integration

**Repository:** `~/project/github.com/subdialia/api-service/`

### 1. KYC Status Query

**File:** `pkg/apiservice/gql/graphql/user/is_kyc.graphql`

```graphql
query GetApprovedKYC ($user_id: uuid!) {
    terrace_schema_id_verification_sessions(
        where: {
            user_id: {_eq: $user_id}
            status: {_in: ["APPROVED"]}
        },
        order_by: {created_at: desc},
        limit: 1
    ) {
        status
    }
}
```

### 2. KYC Approval Check

**File:** `pkg/apiservice/handlers/orders/place_order_v2.go:206-238`

**Purpose:** Enforces KYC requirements before order placement

```go
const StatusIsKycApproved = "APPROVED"

func IsKYCApproved(res *gql.GetApprovedKYCResponse) (approved bool) {
    if len(res.Terrace_schema_id_verification_sessions) == 0 {
        return false
    }
    return res.Terrace_schema_id_verification_sessions[0].GetStatus() == StatusIsKycApproved
}
```

**Regulated Venues Requiring KYC:**
- okx
- bybit
- gate
- binance
- b2c2
- splitter

### 3. KYC Status Caching

**File:** `pkg/apiservice/handlers/orders/kyc_cache.go`

**Caching Strategy:**
- Only caches `APPROVED` status
- 24-hour TTL
- Non-approved statuses always query database (ensures real-time enforcement)

**Cache Key:** `kyc:approved:{user_id}`

### 4. GraphQL Mutations

#### Sync AIPrise Verification
**File:** `pkg/apiservice/gql/apollo/sync_aiprise_verification.graphql`

```graphql
mutation Web_Sync_AIPrise_Verification {
    syncAIPriseVerification {
        ok
        message
        userKyc {
            reviewState
        }
    }
}
```

#### KYC Verification Link Clicked
**File:** `pkg/apiservice/gql/apollo/kyc_verification_link_clicked.graphql`

```graphql
mutation Web_kyc_Verification_Link_Clicked {
    kycVerificationLinkClicked {
        ok
        message
    }
}
```

### 5. GraphQL Types

**File:** `pkg/apiservice/gql/apollo/schema.graphql:6267-6305`

```graphql
type UserKYCDetails {
    reviewState: UserKYCReviewState
    legalFirstName: String
    legalLastName: String
    dateOfBirthUnix: String
    address: Address
    lastSubmissionTimeUnix: String!
    lastReviewTimeUnix: String
    aipriseVerificationUrl: String  # null if completed
}

enum UserKYCReviewState {
    IN_REVIEW
    REJECTED
    APPROVED
    MANUAL_REVIEW
}

input UpdateKYCInput {
    dateOfBirth: DatetimeInput!
    ssn: String
    identificationType: IdentificationType!
    identificationDocumentID: String!
    identificationBackDocumentID: String
    legalFirstName: String!
    legalLastName: String!
    address: AddressInput!
}
```

---

## cefi-lib

**Repository:** `~/project/github.com/subdialia/cefi-lib/`

**Finding:** cefi-lib does NOT contain Aiprise-specific code.

**KYC References:** Only in exchange error code mappings

**Files:**
- `pkg/okx/toolkit.go:251,261,311`
- `pkg/mexc/toolkit.go:258,268,318`

**Context:** Deposit status mapping
```go
depositStatusMap = map[string]string{
    ...
    "14": "KYC limit",  // Exchange-reported KYC limit error
    ...
}
```

This refers to exchange-level KYC limits, not Aiprise integration.

---

## Complete End-to-End Flow

```
1. User navigates to trading page or settings
   ↓
2. Frontend checks KYC status via Hasura GraphQL
   ↓
3. If not verified, shows "Verify Now" banner
   ↓
4. User clicks "Get started"
   ↓
5. Frontend sends STARTED event to id-verification-service
   ↓
6. Session created in database with STARTED status
   ↓
7. Aiprise iframe loads with user data + callback URLs
   ↓
8. User completes verification in Aiprise
   ↓
9. Frontend sends SUCCESSFUL event to id-verification-service
   ↓
10. Database updated to TRIGGERED status
    ↓
11. Aiprise processes verification
    ↓
12. Aiprise sends webhooks to id-verification-service:
    - VERIFICATION_SESSION_STARTED → AIPRISE-STARTED
    - VERIFICATION_REQUEST_SUBMITTED → SUBMITTED
    - INITIAL_CALLBACK with result:
      * APPROVED → APPROVED (user can trade)
      * DECLINED → DECLINED (must retry)
      * REVIEW → IN_REVIEW (manual review)
      * AML hit → FAILED_AML + deactivate user
    ↓
13. Frontend polls Hasura every 5 seconds (while IN_REVIEW)
    ↓
14. Once APPROVED:
    - Frontend shows success banner
    - api-service allows orders on regulated venues
    - Status cached for 24 hours
    ↓
15. Ongoing AML monitoring:
    - Aiprise sends AML_MONITORING_UPDATE webhooks
    - If new hits detected → FAILED_AML + immediate deactivation
```

---

## Security & Compliance

### Authentication & Authorization

1. **Aiprise Callbacks:** HMAC-SHA256 signature verification
   - Header: `X-HMAC-SIGNATURE`
   - Validates request body integrity
   - Prevents unauthorized webhook spoofing

2. **Frontend Events:** JWT validation via FusionAuth
   - Header: `Authorization: Bearer <token>`
   - Validates user identity
   - Ensures only authenticated users can trigger verification

3. **Hasura Queries:** Admin role with secret
   - Uses admin GraphQL client
   - Direct database access for verification records

### AML Enforcement

1. **Immediate User Deactivation:**
   - On AML hit, user immediately deactivated in FusionAuth
   - Prevents re-login and further trading
   - Logged as critical metric (alerts engineering on failure)

2. **Ongoing Monitoring:**
   - Aiprise continuously monitors for new AML hits
   - Updates trigger immediate deactivation
   - No grace period or manual intervention required

3. **Terminal State:**
   - `FAILED_AML` cannot transition to any other state
   - Requires manual investigation and remediation

### Audit Trail

1. **Database Records:**
   - All status changes timestamped
   - `first_callback_at` tracks Aiprise response time
   - `triggered_at` tracks user submission time

2. **Prometheus Metrics:**
   - All events counted and categorized
   - Latency histograms for performance monitoring
   - Failed auth attempts tracked

3. **GitOps Infrastructure:**
   - All configuration changes via git
   - Reviewable, auditable, reversible

---

## Key Files Reference

### id-verification-service
| File | Purpose |
|------|---------|
| `pkg/aiprise/client.go` | Aiprise API client |
| `pkg/rest/aiprise_callback_handler.go` | Main callback router with HMAC auth |
| `pkg/rest/aiprise_callback_handle_initial_callback.go` | Final verification decision handler |
| `pkg/rest/aiprise_callback_handle_case_status_update.go` | Manual status update handler |
| `pkg/rest/aiprise_callback_handle_aml_monitoring_update.go` | Ongoing AML monitoring handler |
| `pkg/rest/event_handler.go` | Frontend event handler |
| `pkg/rest/toolkit.go` | Status state machine & validation |
| `pkg/gql/manager.go` | Hasura GraphQL interface |
| `pkg/fusionauth/client.go` | User deactivation interface |
| `pkg/metrics/counters.go` | Prometheus metrics |
| `pkg/refresh/refresh.go` | CLI refresh command |

### api-service
| File | Purpose |
|------|---------|
| `pkg/apiservice/handlers/orders/place_order_v2.go` | KYC enforcement for order placement |
| `pkg/apiservice/handlers/orders/kyc_cache.go` | Redis caching for KYC status |
| `pkg/apiservice/gql/graphql/user/is_kyc.graphql` | KYC status query |
| `pkg/apiservice/gql/apollo/schema.graphql` | GraphQL type definitions |

### dashboard-service (from previous sections)
| File | Purpose |
|------|---------|
| `component/new/kyc/aiprise-frame.tsx` | Aiprise Web SDK integration |
| `component/new/kyc/kyc-modal.tsx` | KYC modal UI |
| `toolkit/kyc.ts` | KYC utilities & status mapping |
| `rest/sendKycCallbackEvent.ts` | GraphQL mutation for events |
| `stores/AccountStore.ts` | State management with polling |
