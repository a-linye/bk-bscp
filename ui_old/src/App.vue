<template>
  <div class="page-content-container">
    <notice-component v-if="enableNotice" :api-url="noticeApiURL" @show-alert-change="showNotice = $event" />
    <Header v-if="!hideNav"></Header>
    <div :class="['content', { 'show-notice': showNotice }, { 'hide-nav': hideNav }]">
      <router-view></router-view>
      <permission-dialog :show="showApplyPermDialog"></permission-dialog>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, watch } from 'vue';
  import { useRoute } from 'vue-router';
  import { storeToRefs } from 'pinia';
  import useGlobalStore from './store/global';
  import NoticeComponent from '@blueking/notice-component';
  import '@blueking/notice-component/dist/style.css';
  import Header from './components/head.vue';
  import PermissionDialog from './components/permission/apply-dialog.vue';

  const route = useRoute();

  const globalStore = useGlobalStore();
  const { showApplyPermDialog, showNotice, hideNav } = storeToRefs(globalStore);

  // URL 参数 hideNav=1 时隐藏顶部导航栏（用于 iframe 内嵌等场景）。
  // 首次加载从 query 读取后写入全局 store，后续页面间 query 跳转即使不携带该参数也保持隐藏状态；
  // 仅当 query 显式携带 hideNav 参数时才以 query 为准。
  if (route.query.hideNav !== undefined) {
    hideNav.value = route.query.hideNav === '1';
  }
  watch(
    () => route.query.hideNav,
    (val) => {
      if (val !== undefined) {
        hideNav.value = val === '1';
      }
    },
  );

  // @ts-ignore
  const noticeApiURL = `${window.BK_BCS_BSCP_API}/api/v1/announcements`;
  // @ts-ignore
  const enableNotice = window.ENABLE_BK_NOTICE === 'true';

  onMounted(() => {
    globalStore.getAppGlobalConfig();
  });
</script>

<style scoped lang="scss">
  .page-content-container {
    min-width: 1366px;
    overflow: auto;
  }
  .content {
    /* 顶部导航栏与消息通知栏高度，供内部布局统一计算视口偏移，避免隐藏导航栏后留白 */
    --header-height: 52px;
    --notice-height: 0px;
    height: calc(100vh - var(--header-height));
    &.show-notice {
      --notice-height: 40px;
      height: calc(100vh - var(--header-height) - var(--notice-height));
    }
    /* 隐藏顶部导航栏时内容占满整屏 */
    &.hide-nav {
      --header-height: 0px;
      height: 100vh;
      &.show-notice {
        height: calc(100vh - var(--notice-height));
      }
    }
  }
</style>
