import axios from 'axios'

const http = axios.create({ baseURL: '/' })

const KEY_STORAGE = 'modelhub-api-key'

export function getApiKey(): string {
  return localStorage.getItem(KEY_STORAGE) || ''
}

export function setApiKey(key: string): void {
  if (key) {
    localStorage.setItem(KEY_STORAGE, key)
  } else {
    localStorage.removeItem(KEY_STORAGE)
  }
}

export interface ModelInfo {
  id: string
  object: string
  created: number
  owned_by: string
}

export async function listModels(): Promise<ModelInfo[]> {
  const { data } = await http.get('/v1/models', {
    headers: { Authorization: `Bearer ${getApiKey()}` },
  })
  return data.data as ModelInfo[]
}

export function authHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${getApiKey()}`,
    'Content-Type': 'application/json',
  }
}