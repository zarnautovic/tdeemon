// What the user connects: an AI coach (through MCP), an iPhone (Health
// Auto Export), or another of the scopes they pick.
export type ConnectKind = 'coach' | 'phone' | 'other'

export const connectKinds: readonly ConnectKind[] = ['coach', 'phone', 'other']

// The scopes an access key may have, in the server's order.
export const scopes = ['ingest', 'read', 'tools'] as const
export type Scope = (typeof scopes)[number]
