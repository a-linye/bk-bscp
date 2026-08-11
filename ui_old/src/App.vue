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
  import { computed, onMounted } from 'vue';
  import { useRoute } from 'vue-router';
  import { storeToRefs } from 'pinia';
  import useGlobalStore from './store/global';
  import NoticeComponent from '@blueking/notice-component';
  import '@blueking/notice-component/dist/style.css';
  import Header from './components/head.vue';
  import PermissionDialog from './components/permission/apply-dialog.vue';

  const route = useRoute();

  const globalStore = useGlobalStore();
  const { showApplyPermDialog, showNotice } = storeToRefs(globalStore);

  // URL 参数 hideNav=1 时隐藏顶部导航栏（用于 iframe 内嵌等场景）
  const hideNav = computed(() => route.query.hideNav === '1');

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
    height: calc(100vh - 52px);
    &.show-notice {
      height: calc(100vh - 92px);
    }
    /* 隐藏顶部导航栏时内容占满整屏 */
    &.hide-nav {
      height: 100vh;
      &.show-notice {
        height: calc(100vh - 40px);
      }
    }
  }
</style>
