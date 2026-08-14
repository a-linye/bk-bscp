<template>
  <section :class="['version-layout-container', { 'show-notice': showNotice, 'without-footer': !showFooter }]">
    <section class="layout-header">
      <slot name="header"></slot>
    </section>
    <section class="layout-content">
      <slot></slot>
    </section>
    <section v-if="showFooter" class="layout-footer">
      <slot name="footer"></slot>
    </section>
  </section>
</template>
<script setup lang="ts">
  import { storeToRefs } from 'pinia';
  import useGlobalStore from '../../../../../../store/global';

  const { showNotice } = storeToRefs(useGlobalStore());

  withDefaults(
    defineProps<{
      showFooter?: boolean;
    }>(),
    {
      showFooter: true,
    },
  );
</script>
<style lang="scss" scoped>
  .version-layout-container {
    position: fixed;
    top: calc(var(--header-height, 52px) + var(--notice-height, 0px));
    left: 0;
    bottom: 0;
    right: 0;
    height: calc(100vh - var(--header-height, 52px) - var(--notice-height, 0px));
    background: #ffffff;
    z-index: 2000;
    &.show-notice {
      &.without-footer {
        height: calc(100vh - var(--header-height, 52px) - var(--notice-height, 0px) + 4px);
      }
    }
    &.without-footer {
      .layout-content {
        height: calc(100% - 48px);
      }
    }
    .layout-header {
      position: relative;
      height: 52px;
      box-shadow: 0 3px 4px 0 rgba(0, 0, 0, 0.04);
      z-index: 1;
    }
    .layout-content {
      height: calc(100% - 100px);
      background: #f5f7fa;
      overflow: auto;
    }
    .layout-footer {
      height: 48px;
      border-top: 1px solid #dcdee5;
    }
  }
</style>
