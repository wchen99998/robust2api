import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

const useAppStore = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  })
  return vi.fn()
})

vi.mock('@/stores/app', () => ({
  useAppStore
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import DashboardView from '../DashboardView.vue'

describe('admin DashboardView', () => {
  it('embeds the provisioned Grafana dashboard when grafana_url is configured', () => {
    useAppStore.mockReturnValue({
      cachedPublicSettings: { grafana_url: 'https://grafana.example.com' },
      grafanaUrl: ''
    })

    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          EmptyState: { template: '<div><slot name="icon" /></div>' },
          Icon: true
        }
      }
    })

    const iframe = wrapper.get('iframe')
    const dashboardUrl = 'https://grafana.example.com/d/robust2api-admin-overview/robust2api-admin-overview'
    expect(iframe.attributes('src')).toBe(dashboardUrl)
    expect(wrapper.get('a').attributes('href')).toBe(dashboardUrl)
  })

  it('normalizes grafana_url when an existing /d route is provided', () => {
    useAppStore.mockReturnValue({
      cachedPublicSettings: { grafana_url: 'https://grafana.example.com/grafana/d/legacy/old' },
      grafanaUrl: ''
    })

    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          EmptyState: { template: '<div><slot name="icon" /></div>' },
          Icon: true
        }
      }
    })

    expect(wrapper.get('iframe').attributes('src')).toBe(
      'https://grafana.example.com/grafana/d/robust2api-admin-overview/robust2api-admin-overview'
    )
  })

  it('shows the empty state when grafana_url is missing', () => {
    useAppStore.mockReturnValue({
      cachedPublicSettings: { grafana_url: '' },
      grafanaUrl: ''
    })

    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          EmptyState: {
            props: ['title', 'description'],
            template: '<div class="empty-state">{{ title }}|{{ description }}</div>'
          },
          Icon: true
        }
      }
    })

    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('.empty-state').text()).toContain('admin.dashboard.grafanaMissingDescription')
  })

  it('shows invalid empty state when grafana_url is malformed', () => {
    useAppStore.mockReturnValue({
      cachedPublicSettings: { grafana_url: 'javascript:alert(1)' },
      grafanaUrl: ''
    })

    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          EmptyState: {
            props: ['title', 'description'],
            template: '<div class="empty-state">{{ title }}|{{ description }}</div>'
          },
          Icon: true
        }
      }
    })

    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('.empty-state').text()).toContain('admin.dashboard.grafanaInvalidDescription')
  })
})
