import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockGet = vi.fn()

vi.mock('../../client', () => ({
  apiClient: {
    get: (...args: unknown[]) => mockGet(...args),
  },
}))

import systemAPI, { getVersion } from '../system'

describe('admin/system API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('only exposes getVersion in systemAPI', () => {
    expect(Object.keys(systemAPI)).toEqual(['getVersion'])
  })

  it('getVersion requests /admin/system/version', async () => {
    mockGet.mockResolvedValue({ data: { version: '1.2.3' } })

    const result = await getVersion()

    expect(mockGet).toHaveBeenCalledWith('/admin/system/version')
    expect(result).toEqual({ version: '1.2.3' })
  })
})
