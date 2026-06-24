#!/bin/bash
# Pre-commit 代码评审检查脚本
# 用于在提交前进行基础检查并提醒进行 AI 代码评审

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Pre-commit 代码变更摘要"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 获取变更统计
echo "📊 变更统计:"
git diff --cached --stat
echo ""

# 获取变更文件列表
echo "📁 变更文件:"
git diff --cached --name-only
echo ""

# 检查是否有调试代码
echo "🔍 检查调试代码..."
DEBUG_LINES=$(git diff --cached | grep -E "^\+" | grep -E "(console\.log|debugger|print\()" | head -5 || true)
if [ -n "$DEBUG_LINES" ]; then
    echo "⚠️  发现可能的调试代码:"
    echo "$DEBUG_LINES"
    echo ""
fi

# 检查是否有敏感文件
echo "🔍 检查敏感文件..."
SENSITIVE_FILES=$(git diff --cached --name-only | grep -E "(\.env|credentials|secret|password|\.pem|\.key)" | head -5 || true)
if [ -n "$SENSITIVE_FILES" ]; then
    echo "🔴 发现可能的敏感文件:"
    echo "$SENSITIVE_FILES"
    echo ""
    echo "请确认是否应该提交这些文件！"
    echo ""
fi

# 检查变更行数
LINES_CHANGED=$(git diff --cached --numstat | awk '{sum+=$1+$2} END {print sum}')
if [ -n "$LINES_CHANGED" ] && [ "$LINES_CHANGED" -gt 400 ]; then
    echo "⚠️  变更行数较多 ($LINES_CHANGED 行)，建议拆分成更小的提交"
    echo ""
fi

# 检查是否有 TODO/FIXME
TODO_LINES=$(git diff --cached | grep -E "^\+" | grep -E "(TODO|FIXME|XXX|HACK)" | head -5 || true)
if [ -n "$TODO_LINES" ]; then
    echo "📝 发现 TODO/FIXME 标记:"
    echo "$TODO_LINES"
    echo ""
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "💡 建议在 Cursor 中运行: /code-review staged"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 如果发现严重问题，可以选择阻止提交
# 取消下面的注释可以阻止包含敏感文件的提交
# if [ -n "$SENSITIVE_FILES" ]; then
#     echo "❌ 提交已阻止：发现敏感文件"
#     exit 1
# fi

exit 0
