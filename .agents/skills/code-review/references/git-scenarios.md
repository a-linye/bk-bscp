# Git 场景评审指南

针对不同的 Git 场景，使用对应的命令和流程进行代码评审。

## 分支合并评审 (merge)

**使用场景**：在将 feature 分支合并到 main/develop 分支之前，评审所有待合并的代码变更。

**命令格式**：
```bash
/code-review merge <source-branch> <target-branch>
```

**执行流程**：

```bash
# 1. 获取待合并的 commits 列表
git log <target>..<source> --oneline

# 2. 获取完整的代码变更差异
git diff <target>...<source>

# 3. 获取变更的文件列表
git diff <target>...<source> --name-only

# 4. 获取变更统计信息
git diff <target>...<source> --stat
```

**检查要点**：
- 检查所有待合入的 commits 是否符合规范
- 评估变更范围是否合理（建议单次 PR < 400 行变更）
- 识别可能的合并冲突风险
- 确保功能完整性和代码质量

## 指定 Commit 评审 (commit)

**使用场景**：评审特定 commit 的代码变更，用于检查历史提交或特定修改。

**命令格式**：
```bash
/code-review commit <commit-id>
```

**执行流程**：

```bash
# 1. 获取 commit 的详细信息和变更内容
git show <commit-id>

# 2. 获取 commit message
git log -1 --format="%s%n%n%b" <commit-id>

# 3. 获取变更的文件列表
git show <commit-id> --name-only --pretty=format:""

# 4. 获取变更统计
git show <commit-id> --stat
```

**检查要点**：
- Commit message 是否清晰描述了变更内容
- 变更是否符合单一职责原则
- 代码质量是否符合规范

## 最近提交评审 (last-commit)

**使用场景**：快速评审刚刚提交的代码，用于提交后的自查或 review。

**命令格式**：
```bash
/code-review last-commit
```

**执行流程**：

```bash
# 1. 获取最近一次 commit 的 ID
git log -1 --format="%H"

# 2. 获取最近 commit 的完整信息
git show HEAD

# 3. 获取变更文件列表
git show HEAD --name-only --pretty=format:""
```

**检查要点**：
- 与 commit 评审相同
- 特别关注是否有遗漏的文件或调试代码

## 暂存区评审 (staged)

**使用场景**：在执行 `git commit` 之前，评审已经 `git add` 的代码变更。适合作为提交前的最后检查。

**命令格式**：
```bash
/code-review staged
```

**执行流程**：

```bash
# 1. 获取暂存区的代码变更
git diff --cached

# 2. 获取暂存区变更的文件列表
git diff --cached --name-only

# 3. 获取暂存区变更统计
git diff --cached --stat
```

**检查要点**：
- 是否有不应该提交的文件（如 .env、node_modules）
- 是否有调试代码（console.log、debugger）
- 代码是否符合规范和最佳实践

## 工作区评审 (unstaged)

**使用场景**：评审当前工作目录中尚未暂存的代码变更，用于开发过程中的自查。

**命令格式**：
```bash
/code-review unstaged
```

**执行流程**：

```bash
# 1. 获取工作区的代码变更
git diff

# 2. 获取工作区变更的文件列表
git diff --name-only

# 3. 获取工作区变更统计
git diff --stat
```

**检查要点**：
- 开发中的代码是否走在正确的方向
- 是否有明显的设计问题需要调整
- 及时发现潜在问题，避免返工

## 全部变更评审 (changes)

**使用场景**：评审所有未提交的代码变更，包括暂存区和工作区的变更。

**命令格式**：
```bash
/code-review changes
```

**执行流程**：

```bash
# 1. 获取 Git 状态
git status --porcelain

# 2. 获取所有变更（staged + unstaged）
git diff HEAD

# 3. 获取所有变更的文件列表
git diff HEAD --name-only

# 4. 获取变更统计
git diff HEAD --stat
```

**检查要点**：
- 综合 staged 和 unstaged 的检查要点
- 确保所有变更都是有意义的
- 检查是否有遗漏的相关文件

## Git 场景通用检查项

所有 Git 场景评审都应包含以下通用检查：

### 1. 变更范围检查
- 变更是否聚焦于单一目标
- 是否存在不相关的修改混入

### 2. 文件类型检查
- 是否有敏感文件（.env, credentials, keys）
- 是否有应该忽略的文件（node_modules, dist, build）

### 3. 代码质量检查
- 是否有调试代码残留
- 是否有注释掉的代码块
- 是否有 TODO/FIXME 需要处理

### 4. Commit 规范检查（适用于 merge、commit、last-commit）
- Commit message 格式是否正确
- 是否遵循 Conventional Commits 规范

## 工作流最佳实践

推荐的代码提交工作流：

```
1. 开发完成后，暂存变更
   $ git add <files>

2. 运行静态检查
   $ npm run lint

3. 在 Cursor 中进行 AI 代码评审
   /code-review staged

4. 根据评审意见修复问题
   （修复后重复步骤 1-3）

5. 确认无问题后提交
   $ git commit -m "feat: your feature description"

6. 推送前进行分支评审（可选）
   /code-review merge feature/xxx main
   $ git push
```

## 紧急情况处理

### 什么是紧急情况

- 允许一个重要的发布继续，而不是回滚
- 修复一个严重影响生产中用户的 bug
- 处理一个紧迫的法律问题
- 关闭一个主要的安全漏洞

### 紧急情况下的处理

- 审查者应更关心审查的速度和代码的正确性
- 这些评审应该优先于所有其他代码评审
- 紧急情况解决后，应再次查看紧急 CL，进行更彻底的检查

### 什么不是紧急情况

- 想要在本周而不是下周发布（除非确实有严格的发布截止日期）
- 开发人员花了很长时间开发一个特性
- 审查人员不在或在另一个时区
- 今天是周五，想在周末前完成
