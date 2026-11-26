import type { StorybookConfig } from '@storybook/react-vite'

// The design system lives in web/shared and is consumed by all four apps, so the
// stories are colocated with the components rather than with an app.
const config: StorybookConfig = {
  stories: ['../shared/**/*.stories.tsx'],
  addons: [],
  framework: { name: '@storybook/react-vite', options: {} },
  core: { disableTelemetry: true },
}

export default config
