import { reactive } from 'vue'
import { fetchMe, getSession, logout, type MeUser } from './api/client'

interface SessionState {
  user: MeUser | null
  loading: boolean
}

// Single shared session state for the whole app.
const state = reactive<SessionState>({ user: null, loading: false })

export function isLoggedIn(): boolean {
  return getSession() !== null
}

// Loads /api/me when a session token exists. force reloads unconditionally.
export async function loadMe(force = false): Promise<void> {
  if (state.user && !force) return
  if (!getSession()) {
    state.user = null
    return
  }
  state.loading = true
  try {
    state.user = await fetchMe()
  } catch {
    state.user = null
  } finally {
    state.loading = false
  }
}

export async function signOut(): Promise<void> {
  await logout()
  state.user = null
}

export function useSession(): SessionState {
  return state
}