<script setup>
import { countries } from "../geography";
const country = defineModel({ type: String, default: "" });
</script>
<template>
  <el-select
    v-model="country"
    filterable
    clearable
    value-on-clear=""
    placeholder="未设置地区"
    aria-label="国家或地区"
    style="width: 100%"
    ><el-option
      v-for="item in countries"
      :key="item.code"
      :label="`${item.name} · ${item.code}`"
      :value="item.code"
  /></el-select>
</template>
