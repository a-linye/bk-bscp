import { ref, computed } from 'vue';
import { storeToRefs } from 'pinia';
import useGlobalStore from '../../store/global';
import { getCachedProjectList } from '../project';
import type { IProjectItem } from '../../../types/project';

/**
 * 统一对外展示"业务名 / 项目名"文本
 * 业务名取自全局 store 的 spaceList，项目名复用在 utils/project 中的模块级缓存，
 * 避免重复请求，且与 biz-project-selector 组件使用同一份数据源
 * @param separator 业务与项目之间的分隔符，默认 ' / '
 */
export default function useBizProjectText(separator = ' / ') {
  const globalStore = useGlobalStore();
  const { spaceId, projectId, spaceList } = storeToRefs(globalStore);

  // 各空间的项目列表缓存（按 spaceId 维度，复用 getCachedProjectList 的模块级缓存）
  const projectMap = ref<Record<string, IProjectItem[]>>({});

  // 确保当前空间的项目列表已加载（已缓存则直接复用，不发请求）
  const ensureProjectList = async () => {
    if (!spaceId.value) return;
    const list = await getCachedProjectList(spaceId.value);
    projectMap.value = {
      ...projectMap.value,
      [spaceId.value]: list,
    };
  };

  // 业务名 / 项目名，无项目态时仅显示业务名，与 biz-project-selector 行为一致
  const bizProjectText = computed(() => {
    const biz = spaceList.value.find((b) => b.space_id === spaceId.value);
    if (!biz) return '';
    const project = (projectMap.value[spaceId.value] || [])
      .find((p) => String(p.id) === projectId.value);
    return project
      ? `${biz.space_name}${separator}${project.spec.name}`
      : biz.space_name;
  });

  return {
    bizProjectText,
    ensureProjectList,
  };
}
