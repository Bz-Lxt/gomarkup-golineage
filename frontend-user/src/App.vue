<script setup lang="ts">
import { onMounted, ref } from 'vue'

import AppToast from '@/components/base/AppToast.vue'
import CreateEdgeModal from '@/components/CreateEdgeModal.vue'
import CreateNodeModal from '@/components/CreateNodeModal.vue'
import GraphCanvas from '@/components/GraphCanvas.vue'
import InspectorDrawer from '@/components/InspectorDrawer.vue'
import LeftPanel from '@/components/LeftPanel.vue'
import TimelineBar from '@/components/TimelineBar.vue'
import TopBar from '@/components/TopBar.vue'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'

const graph = useGraphStore()
const timeline = useTimelineStore()

const createNodeOpen = ref(false)
const createEdgeOpen = ref(false)

onMounted(async () => {
  await graph.bootstrap()
  await Promise.all([timeline.loadBounds(), timeline.loadEvents()])
})
</script>

<template>
  <div class="flex h-full w-full flex-col bg-void">
    <TopBar />
    <div class="flex min-h-0 min-w-0 flex-1">
      <LeftPanel @create-node="createNodeOpen = true" @create-edge="createEdgeOpen = true" />
      <GraphCanvas />
      <InspectorDrawer />
    </div>
    <TimelineBar />
    <CreateNodeModal :open="createNodeOpen" @close="createNodeOpen = false" />
    <CreateEdgeModal :open="createEdgeOpen" @close="createEdgeOpen = false" />
    <AppToast />
  </div>
</template>
