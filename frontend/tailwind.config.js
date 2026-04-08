/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        canvas: {
          DEFAULT: '#f2f0ed',
          dark: '#1c1b1a'
        },
        surface: {
          DEFAULT: '#ffffff',
          solid: '#ffffff',
          'solid-dark': '#2c2c2e'
        },
        'mica-text': {
          primary: '#1d1d1f',
          secondary: '#6e6e73',
          tertiary: '#aeaeb2',
          'primary-dark': '#f5f5f7',
          'secondary-dark': '#a1a1a6',
          'tertiary-dark': '#636366'
        },
        accent: {
          DEFAULT: '#1d1d1f',
          fg: '#ffffff',
          dark: '#f5f5f7',
          'fg-dark': '#1c1b1a'
        },
        'status-blue': { DEFAULT: '#007aff', dark: '#0a84ff' },
        'status-green': { DEFAULT: '#34c759', dark: '#30d158' },
        'status-red': { DEFAULT: '#ff3b30', dark: '#ff453a' },
        'status-amber': { DEFAULT: '#ff9500', dark: '#ff9f0a' },
        primary: {
          50: '#f0f5ff', 100: '#e0ebff', 200: '#c2d6ff', 300: '#85adff',
          400: '#4d88ff', 500: '#007aff', 600: '#0066d6', 700: '#004dad',
          800: '#003380', 900: '#001a52', 950: '#000d29'
        },
        dark: {
          50: '#f8f8f7', 100: '#f2f0ed', 200: '#e5e3e0', 300: '#c7c5c2',
          400: '#a1a1a6', 500: '#6e6e73', 600: '#48484a', 700: '#3a3a3c',
          800: '#2c2c2e', 900: '#1c1b1a', 950: '#141413'
        }
      },
      fontFamily: {
        sans: [
          '-apple-system', 'BlinkMacSystemFont', 'SF Pro Display', 'SF Pro Text',
          'system-ui', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial',
          'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'sans-serif'
        ],
        mono: [
          'SF Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo',
          'Monaco', 'Consolas', 'monospace'
        ],
        serif: ["'New York'", "'Iowan Old Style'", 'Georgia', "'Times New Roman'", 'serif']
      },
      fontSize: {
        'mica-caption': ['11px', { lineHeight: '1.2', fontWeight: '500', letterSpacing: '0.3px' }],
        'mica-subhead': ['13px', { lineHeight: '1.4', fontWeight: '500' }],
        'mica-body': ['15px', { lineHeight: '1.5', fontWeight: '400' }],
        'mica-headline': ['15px', { lineHeight: '1.3', fontWeight: '600' }],
        'mica-title2': ['20px', { lineHeight: '1.3', fontWeight: '600' }],
        'mica-title1': ['24px', { lineHeight: '1.2', fontWeight: '600', letterSpacing: '-0.3px' }],
        'mica-large-title': ['34px', { lineHeight: '1.1', fontWeight: '600', letterSpacing: '-0.5px' }]
      },
      borderRadius: {
        'mica-sm': '6px',
        'mica': '8px',
        'mica-lg': '12px',
        'mica-xl': '16px'
      },
      boxShadow: {
        'mica-popover': '0 4px 24px rgba(0, 0, 0, 0.08)',
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)'
      },
      animation: {
        'fade-in': 'fadeIn 0.25s ease-out',
        'slide-up': 'slideUp 0.25s ease-out',
        'slide-down': 'slideDown 0.25s ease-out',
        'slide-in-right': 'slideInRight 0.25s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        shimmer: 'shimmer 2s linear infinite'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(8px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-8px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(16px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.96)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        }
      }
    }
  },
  plugins: []
}
