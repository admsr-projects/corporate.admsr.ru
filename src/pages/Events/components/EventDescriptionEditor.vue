<script setup lang="ts">
/**
 * Описание мероприятия — тот же редактор, что у новостей (ADR-055): UEditor с панелью
 * и эмодзи, Markdown-сокращения при вводе («## », «- », «**…**»). Хранится HTML.
 */
import { newsEditorToolbarItems } from '../../../composables/newsEditorToolbar';
import { newsEditorEmojiMenuItems, newsEditorExtensions } from '../../../composables/newsEditorExtensions';
import { newsEditorHandlers } from '../../../composables/newsEditorHandlers';
import { newsEditorSlideoverUi } from '../../../composables/newsEditorSlideoverUi';

const model = defineModel<string>({ default: '' });

const appendEditorEmojiTo = () => document.body;
</script>

<template>
  <UEditor
    v-slot="{ editor }"
    v-model="model"
    content-type="html"
    :extensions="newsEditorExtensions"
    :handlers="newsEditorHandlers"
    :ui="newsEditorSlideoverUi"
    placeholder="Расскажите, что будет, для кого и как подготовиться…"
    class="w-full min-h-56 rounded-lg border border-default overflow-hidden"
  >
    <UEditorEmojiMenu :editor="editor" :items="newsEditorEmojiMenuItems" :append-to="appendEditorEmojiTo" />
    <UEditorToolbar
      :editor="editor"
      :items="newsEditorToolbarItems"
      class="sticky top-0 z-10 border-b border-default bg-default/95 backdrop-blur-sm px-2 py-1.5 overflow-x-auto"
    />
  </UEditor>
</template>
