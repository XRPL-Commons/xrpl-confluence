import type { Theme } from 'vitepress';
import DefaultTheme from 'vitepress/theme';
import CopyOrDownloadAsMarkdownButtons from 'vitepress-plugin-llms/vitepress-components/CopyOrDownloadAsMarkdownButtons.vue';
import DownloadLLMsFullDoc from './DownloadLLMsFullDoc.vue';
import './custom.css';

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('CopyOrDownloadAsMarkdownButtons', CopyOrDownloadAsMarkdownButtons);
    app.component('DownloadLLMsFullDoc', DownloadLLMsFullDoc);
  },
} satisfies Theme;
