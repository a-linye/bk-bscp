# -*- coding: utf-8 -*-
"""
AST visitor for Mako template safety checking
参考原项目：bk-process-config-manager/apps/utils/mako_utils/visitor.py

通过遍历抽象语法树（AST）来检查模板中是否使用了危险的操作
"""

import ast

from .exceptions import ForbiddenMakoTemplateException


def _optional_ast_node_types(*names):
    return tuple(getattr(ast, name) for name in names if hasattr(ast, name))


class MakoNodeVisitor(ast.NodeVisitor):
    """
    遍历语法树节点，只放行业务模板需要的语法和调用

    参考原项目：bk-process-config-manager/apps/utils/mako_utils/visitor.py
    """

    # 允许导入的模块。实际可访问成员由 WHITE_LIST_MODULE_CALLS/WHITE_LIST_MODULE_ATTRS 继续收窄。
    WHITE_LIST_MODULES = {
        "datetime",
        "re",
        "random",
        "json",
        "math",
    }

    WHITE_LIST_MODULE_CALLS = {
        "datetime": {
            ("date",),
            ("date", "today"),
            ("datetime",),
            ("datetime", "now"),
            ("datetime", "utcnow"),
            ("datetime", "strptime"),
            ("timedelta",),
        },
        "json": {
            ("dumps",),
            ("loads",),
        },
        "math": {
            ("acos",),
            ("asin",),
            ("atan",),
            ("atan2",),
            ("ceil",),
            ("cos",),
            ("degrees",),
            ("exp",),
            ("fabs",),
            ("floor",),
            ("fmod",),
            ("fsum",),
            ("hypot",),
            ("log",),
            ("log10",),
            ("pow",),
            ("radians",),
            ("sin",),
            ("sqrt",),
            ("tan",),
            ("trunc",),
        },
        "random": {
            ("choice",),
            ("choices",),
            ("randint",),
            ("random",),
            ("randrange",),
            ("sample",),
            ("shuffle",),
            ("uniform",),
        },
        "re": {
            ("compile",),
            ("fullmatch",),
            ("match",),
            ("search",),
            ("split",),
            ("sub",),
        },
    }

    WHITE_LIST_MODULE_ATTRS = {
        "datetime": {
            ("date",),
            ("datetime",),
            ("timedelta",),
        },
        "math": {
            ("e",),
            ("inf",),
            ("nan",),
            ("pi",),
            ("tau",),
        },
    }

    # 业务模板常用的基础函数。未列出的 builtin 即使 Python 可用，也不能在模板中调用。
    WHITE_LIST_FUNCTIONS = {
        "abs",
        "bool",
        "dict",
        "enumerate",
        "float",
        "int",
        "zip",
        "len",
        "list",
        "map",
        "max",
        "min",
        "range",
        "round",
        "set",
        "sorted",
        "str",
        "sum",
        "tuple",
    }

    # 业务模板允许调用的方法。
    WHITE_LIST_METHODS = {
        "add",
        "append",
        "find",
        "findall",
        "format",
        "group",
        "get",
        "index",
        "isdigit",
        "items",
        "iteritems",
        "join",
        "keys",
        "lower",
        "lstrip",
        "now",
        "extend",
        "replace",
        "setdefault",
        "update",
        "split",
        "sort",
        "startswith",
        "strftime",
        "strip",
        "timestamp",
        "values",
        "xpath",
    }

    WHITE_LIST_LAMBDA_METHODS = {
        "format",
        "lower",
    }

    WHITE_LIST_LAMBDA_FUNCTIONS = {
        "bool",
        "float",
        "int",
        "str",
    }

    WHITE_LIST_LAMBDA_BINOPS = (
        ast.Add,
        ast.Sub,
        ast.Mult,
        ast.Div,
        ast.FloorDiv,
        ast.Mod,
    )

    WHITE_LIST_LAMBDA_COMPARE_OPS = (
        ast.Eq,
        ast.NotEq,
        ast.Lt,
        ast.LtE,
        ast.Gt,
        ast.GtE,
        ast.In,
        ast.NotIn,
    )

    # 仅允许模板显式抛出的异常类型（raise Exception(...) / raise ValueError(...)）。
    WHITE_LIST_EXCEPTIONS = {
        "Exception",
        "TypeError",
        "ValueError",
    }

    # 业务模板允许访问的数据属性。
    WHITE_LIST_ATTRS = {
        "attrib",
        "cc_host",
        "cc_module",
        "cc_set",
    }

    # Mako 表达式过滤器会在表达式 AST 之外执行，只允许内置的纯转义/转换过滤器。
    WHITE_LIST_FILTERS = {
        "entity",
        "h",
        "n",
        "str",
        "trim",
        "u",
        "unicode",
        "x",
    }

    # 明确禁止的名称。普通上下文变量默认允许，但这些名称不能作为变量或函数出现。
    FORBIDDEN_NAMES = {
        "__import__",
        "breakpoint",
        "capture",
        "compile",
        "context",
        "delattr",
        "dir",
        "eval",
        "exec",
        "execfile",
        "exit",
        "getattr",
        "globals",
        "hasattr",
        "help",
        "input",
        "iter",
        "locals",
        "memoryview",
        "next",
        "octal",
        "open",
        "print",
        "quit",
        "self",
        "setattr",
        "super",
        "vars",
    }

    FORBIDDEN_NODE_TYPES = (
        ast.AsyncFunctionDef,
        ast.AsyncWith,
        ast.Await,
        ast.ClassDef,
        ast.Delete,
        ast.DictComp,
        ast.GeneratorExp,
        ast.Global,
        ast.Nonlocal,
        ast.SetComp,
        ast.With,
        ast.Yield,
        ast.YieldFrom,
        *_optional_ast_node_types("Match", "NamedExpr", "TryStar", "TypeAlias"),
    )

    def __init__(self, white_list_modules=None):
        """
        初始化节点访问器
        
        Args:
            white_list_modules: 自定义白名单模块列表（默认使用类属性）
        """
        self.white_list_modules = set(white_list_modules or self.WHITE_LIST_MODULES)
        self.white_list_filters = set(self.WHITE_LIST_FILTERS)
        self.allowed_module_bindings = {}
        self.allowed_import_bindings = {}
        self.allowed_template_functions = set()
        self.allowed_lambda_bindings = set()
        self._template_function_defs = {}
        self._template_function_call_check_stack = set()
        self._function_def_depth = 0
        self._module_template_function_stack = []
        self._module_allowed_module_binding_stack = []
        self._module_allowed_import_binding_stack = []
        self._allow_template_function_def_stack = []
        self._mako_control_depth = 0
        self._mako_control_binding_stack = []
        self._comprehension_depth = 0

    def _reject(self, message):
        raise ForbiddenMakoTemplateException(message)

    def _is_dunder(self, name):
        return name.startswith("__") and name.endswith("__")

    def _attribute_parts(self, node):
        parts = []
        while isinstance(node, ast.Attribute):
            parts.append(node.attr)
            node = node.value
        if not isinstance(node, ast.Name):
            return "", ()
        parts.reverse()
        return node.id, tuple(parts)

    def _module_member_path(self, node):
        if isinstance(node, ast.Name) and node.id in self.allowed_import_bindings:
            return self.allowed_import_bindings[node.id]
        root, parts = self._attribute_parts(node)
        if root in self.allowed_module_bindings:
            return self.allowed_module_bindings[root], parts
        if root in self.allowed_import_bindings:
            module_name, base_parts = self.allowed_import_bindings[root]
            return module_name, base_parts + parts
        return "", ()

    def _validate_module_member_path(self, module_name, member_path):
        if not module_name or not member_path:
            return
        for part in member_path:
            if part.startswith("_"):
                self._reject("发现非法属性使用:[{}]，请修改".format(part))

    def _module_allowed_paths(self, module_name):
        return (
            self.WHITE_LIST_MODULE_CALLS.get(module_name, set())
            | self.WHITE_LIST_MODULE_ATTRS.get(module_name, set())
        )

    def _is_allowed_module_attr_path(self, module_name, member_path):
        self._validate_module_member_path(module_name, member_path)
        allowed_paths = self._module_allowed_paths(module_name)
        return any(path[: len(member_path)] == member_path for path in allowed_paths)

    def _is_allowed_module_call_path(self, module_name, member_path):
        self._validate_module_member_path(module_name, member_path)
        return member_path in self.WHITE_LIST_MODULE_CALLS.get(module_name, set())

    def _allowed_callable_import_binding(self, node):
        module_name, member_path = self._module_member_path(node)
        if module_name and self._is_allowed_module_call_path(module_name, member_path):
            return module_name, member_path
        return None

    def _bind_callable_alias_target(self, target, binding):
        if binding is None:
            return
        if isinstance(target, ast.Name):
            self.allowed_import_bindings[target.id] = binding

    def _bind_lambda_target(self, target, value):
        if isinstance(value, ast.Lambda) and isinstance(target, ast.Name):
            self.allowed_lambda_bindings.add(target.id)

    def _validate_random_shuffle_call(self, node):
        if len(node.args) != 1 or node.keywords:
            self._reject("发现非法函数调用:[shuffle]，请修改")
        arg = node.args[0]
        if isinstance(arg, ast.List):
            for element in arg.elts:
                self.visit(element)
            return
        if isinstance(arg, ast.Name):
            if (
                arg.id in ("this", "cc")
                or arg.id in self.allowed_module_bindings
                or arg.id in self.allowed_import_bindings
                or arg.id in self.allowed_template_functions
                or arg.id in self.allowed_lambda_bindings
            ):
                self._reject("发现非法函数调用:[shuffle]，请修改")
            self._validate_binding_name(arg.id)
            return
        self._reject("发现非法函数调用:[shuffle]，请修改")

    def _validate_binding_name(self, name):
        if self._is_dunder(name) or name in self.FORBIDDEN_NAMES:
            self._reject("发现非法名称使用:[{}]，请修改".format(name))

    def validate_filter_name(self, name):
        if name not in self.white_list_filters:
            self._reject("发现非法过滤器使用:[{}]，请修改".format(name))

    def _visit_function_annotations(self, node):
        for arg in (
            list(getattr(node.args, "posonlyargs", []))
            + list(node.args.args)
            + list(node.args.kwonlyargs)
        ):
            if arg.annotation is not None:
                self.visit(arg.annotation)

        for arg in (node.args.vararg, node.args.kwarg):
            if arg is not None and arg.annotation is not None:
                self.visit(arg.annotation)

        if node.returns is not None:
            self.visit(node.returns)

    def _function_argument_names(self, node):
        names = set()
        args = (
            list(getattr(node.args, "posonlyargs", []))
            + list(node.args.args)
            + list(node.args.kwonlyargs)
        )
        for arg in args:
            self._validate_binding_name(arg.arg)
            names.add(arg.arg)

        for arg in (node.args.vararg, node.args.kwarg):
            if arg is not None:
                self._validate_binding_name(arg.arg)
                names.add(arg.arg)

        return names

    def _collect_function_local_bindings(self, node):
        names = self._function_argument_names(node)
        for stmt in node.body:
            for child in ast.walk(stmt):
                if isinstance(child, ast.Name) and isinstance(child.ctx, ast.Store):
                    names.add(child.id)
                    continue
                if isinstance(child, ast.Import):
                    for alias in child.names:
                        names.add(alias.asname or alias.name.split(".", 1)[0])
                    continue
                if isinstance(child, ast.ImportFrom):
                    for alias in child.names:
                        if alias.name != "*":
                            names.add(alias.asname or alias.name)
        return names

    def _remove_allowed_bindings(self, names):
        for name in names:
            self.allowed_module_bindings.pop(name, None)
            self.allowed_import_bindings.pop(name, None)
            self.allowed_template_functions.discard(name)
            self.allowed_lambda_bindings.discard(name)

    def _remove_binding_names(self, names, module_bindings, import_bindings, template_functions):
        for name in names:
            module_bindings.pop(name, None)
            import_bindings.pop(name, None)
            template_functions.discard(name)

    def _module_import_binding(self, name):
        module_name = name.name.split(".", 1)[0]
        if name.name != module_name or module_name not in self.white_list_modules:
            self._reject("发现非法导入:[{}]，请修改".format(name.name))
        binding_name = name.asname or module_name
        self._validate_binding_name(binding_name)
        return binding_name, module_name

    def _import_from_module_name(self, node):
        module_name = node.module or ""
        if node.level != 0 or module_name not in self.white_list_modules:
            self._reject("发现非法导入:[{}]，请修改".format(node.module or ""))
        return module_name

    def _from_import_binding(self, module_name, name):
        if name.name == "*" or name.name.startswith("_"):
            self._reject("发现非法导入:[{}]，请修改".format(name.name))
        binding_name = name.asname or name.name
        member_path = (name.name,)
        self._validate_binding_name(binding_name)
        if not self._is_allowed_module_attr_path(module_name, member_path):
            self._reject("发现非法导入:[{}]，请修改".format(name.name))
        return binding_name, member_path

    def _current_module_template_functions(self):
        if not self._module_template_function_stack:
            return set()
        return self._module_template_function_stack[-1]

    def _current_module_allowed_module_bindings(self):
        if not self._module_allowed_module_binding_stack:
            return {}
        return self._module_allowed_module_binding_stack[-1]

    def _current_module_allowed_import_bindings(self):
        if not self._module_allowed_import_binding_stack:
            return {}
        return self._module_allowed_import_binding_stack[-1]

    def _validate_template_function_current_bindings(self, name):
        if self._function_def_depth > 0 or name in self._template_function_call_check_stack:
            return
        node = self._template_function_defs.get(name)
        if node is None:
            return

        outer_module_bindings = self.allowed_module_bindings
        outer_import_bindings = self.allowed_import_bindings
        outer_template_functions = self.allowed_template_functions
        outer_lambda_bindings = self.allowed_lambda_bindings
        self._template_function_call_check_stack.add(name)
        self._function_def_depth += 1
        try:
            self.allowed_module_bindings = dict(outer_module_bindings)
            self.allowed_import_bindings = dict(outer_import_bindings)
            self.allowed_template_functions = set(outer_template_functions)
            self.allowed_lambda_bindings = set(outer_lambda_bindings)
            self._remove_allowed_bindings(self._collect_function_local_bindings(node))
            for stmt in node.body:
                self.visit(stmt)
        finally:
            self.allowed_module_bindings = outer_module_bindings
            self.allowed_import_bindings = outer_import_bindings
            self.allowed_template_functions = outer_template_functions
            self.allowed_lambda_bindings = outer_lambda_bindings
            self._function_def_depth -= 1
            self._template_function_call_check_stack.discard(name)

    def _statement_binding_names(self, node):
        names = set()
        for child in ast.walk(node):
            if isinstance(child, ast.Name) and isinstance(child.ctx, ast.Store):
                names.add(child.id)
                continue
            if isinstance(child, ast.Import):
                for alias in child.names:
                    names.add(alias.asname or alias.name.split(".", 1)[0])
                continue
            if isinstance(child, ast.ImportFrom):
                for alias in child.names:
                    if alias.name != "*":
                        names.add(alias.asname or alias.name)
                continue
            if isinstance(child, ast.FunctionDef):
                names.add(child.name)
        return names

    def _can_publish_template_function(self):
        return (
            self._allow_template_function_def_stack
            and self._allow_template_function_def_stack[-1]
        )

    def _binding_state_changed_names(self, before):
        (
            before_module_bindings,
            before_import_bindings,
            before_template_functions,
            before_lambda_bindings,
        ) = before
        names = (
            set(before_module_bindings)
            | set(before_import_bindings)
            | set(before_template_functions)
            | set(before_lambda_bindings)
            | set(self.allowed_module_bindings)
            | set(self.allowed_import_bindings)
            | set(self.allowed_template_functions)
            | set(self.allowed_lambda_bindings)
        )
        changed_names = set()
        for name in names:
            before_state = (
                before_module_bindings.get(name),
                before_import_bindings.get(name),
                name in before_template_functions,
                name in before_lambda_bindings,
            )
            current_state = (
                self.allowed_module_bindings.get(name),
                self.allowed_import_bindings.get(name),
                name in self.allowed_template_functions,
                name in self.allowed_lambda_bindings,
            )
            if before_state != current_state:
                changed_names.add(name)
        return changed_names

    def enter_mako_control(self):
        self._mako_control_binding_stack.append(
            (
                dict(self.allowed_module_bindings),
                dict(self.allowed_import_bindings),
                set(self.allowed_template_functions),
                set(self.allowed_lambda_bindings),
            )
        )
        self._mako_control_depth += 1

    def exit_mako_control(self):
        before = self._mako_control_binding_stack.pop()
        changed_names = self._binding_state_changed_names(before)
        (
            self.allowed_module_bindings,
            self.allowed_import_bindings,
            self.allowed_template_functions,
            self.allowed_lambda_bindings,
        ) = before
        self._remove_allowed_bindings(changed_names)
        self._mako_control_depth -= 1

    def _unbind_target(self, target):
        if isinstance(target, ast.Name):
            self._validate_binding_name(target.id)
            self._remove_allowed_bindings({target.id})
            return

        if isinstance(target, (ast.Tuple, ast.List)):
            for element in target.elts:
                self._unbind_target(element)
            return

        if isinstance(target, ast.Starred):
            self._unbind_target(target.value)
            return

        self._reject("发现非法赋值目标:[{}]，请修改".format(target.__class__.__name__))

    def _validate_dict_mutation_receiver(self, node):
        """setdefault/update 仅允许作用在模板局部 dict，禁止改写 this/cc 拓扑对象。"""
        if isinstance(node, (ast.Dict, ast.List, ast.Tuple)):
            return
        if isinstance(node, ast.Name):
            if node.id in ("this", "cc") or node.id in self.allowed_module_bindings:
                self._reject("发现非法函数调用:[{}]，请修改".format(node.id))
            return
        if isinstance(node, ast.Subscript):
            self._validate_dict_mutation_receiver(node.value)
            return
        if isinstance(node, ast.Call):
            if isinstance(node.func, ast.Attribute) and node.func.attr == "setdefault":
                self._validate_dict_mutation_receiver(node.func.value)
                return
            self._reject("发现非法函数调用:[{}]，请修改".format(node.__class__.__name__))
        root, parts = self._attribute_parts(node)
        if root in ("this", "cc"):
            self._reject("发现非法函数调用:[{}]，请修改".format(root))
        if parts and parts[0] in self.WHITE_LIST_ATTRS:
            self._reject("发现非法函数调用:[{}]，请修改".format(".".join(parts)))

    def _validate_subscript_assign_target(self, node):
        """仅允许对普通变量做下标赋值，禁止改写 this/cc 等对象。"""
        root_name = self._subscript_assign_root_name(node)
        if not root_name:
            self._reject("发现非法赋值目标:[{}]，请修改".format(node.__class__.__name__))
        if (
            root_name in ("this", "cc")
            or root_name in self.allowed_module_bindings
            or root_name in self.allowed_import_bindings
        ):
            self._reject("发现非法赋值目标:[{}]，请修改".format(node.__class__.__name__))
        self._validate_binding_name(root_name)
        self._visit_subscript_assign_slices(node)

    def _subscript_assign_root_name(self, node):
        current = node
        while isinstance(current, ast.Subscript):
            current = current.value
        if isinstance(current, ast.Name):
            return current.id
        if self._is_local_attrib_assign_base(current):
            return current.value.id
        return ""

    def _is_local_attrib_assign_base(self, node):
        return (
            isinstance(node, ast.Attribute)
            and node.attr == "attrib"
            and isinstance(node.value, ast.Name)
            and node.value.id not in ("this", "cc")
            and node.value.id not in self.allowed_module_bindings
            and node.value.id not in self.allowed_import_bindings
        )

    def _visit_subscript_assign_slices(self, node):
        if isinstance(node.value, ast.Subscript):
            self._visit_subscript_assign_slices(node.value)
        elif not isinstance(node.value, ast.Name) and not self._is_local_attrib_assign_base(node.value):
            self._reject("发现非法赋值目标:[{}]，请修改".format(node.__class__.__name__))
        if isinstance(node.slice, ast.Slice):
            if node.slice.lower is not None:
                self.visit(node.slice.lower)
            if node.slice.upper is not None:
                self.visit(node.slice.upper)
            if node.slice.step is not None:
                self.visit(node.slice.step)
            return
        self.visit(node.slice)

    def _visit_assign_target(self, target):
        if isinstance(target, ast.Subscript):
            self._validate_subscript_assign_target(target)
            return
        self._unbind_target(target)

    def _is_allowed_exception_call(self, node):
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name):
            if node.func.id in self.WHITE_LIST_EXCEPTIONS:
                return True
        return False

    def _validate_raise_exc(self, node):
        if self._is_allowed_exception_call(node):
            self.generic_visit(node)
            return
        if (
            isinstance(node, ast.BinOp)
            and isinstance(node.op, ast.Mod)
            and self._is_allowed_exception_call(node.left)
        ):
            self.generic_visit(node.left)
            self.visit(node.right)
            return
        self._reject("发现非法语法使用:[raise]，请修改")

    def _validate_lambda_slice(self, node, param_names):
        if isinstance(node, ast.Tuple):
            for element in node.elts:
                self._validate_lambda_slice(element, param_names)
            return
        if isinstance(node, ast.Constant):
            return
        if isinstance(node, ast.Name) and node.id in param_names:
            return
        self._reject("发现非法语法使用:[Lambda下标]，请修改")

    def _validate_lambda_body(self, node, param_names):
        """仅允许 sorted(key=...) 等简单取值和基础类型转换。"""
        if isinstance(node, ast.Constant):
            return
        if isinstance(node, ast.Name):
            if node.id in param_names or node.id in ("True", "False", "None"):
                return
            self._validate_binding_name(node.id)
            return
        if isinstance(node, ast.Subscript):
            self._validate_lambda_body(node.value, param_names)
            self._validate_lambda_slice(node.slice, param_names)
            return
        if isinstance(node, ast.Attribute):
            if self._is_dunder(node.attr):
                self._reject("发现非法语法使用:[Lambda属性]，请修改")
            self._validate_lambda_body(node.value, param_names)
            return
        if isinstance(node, (ast.List, ast.Tuple)):
            for element in node.elts:
                self._validate_lambda_body(element, param_names)
            return
        if isinstance(node, ast.IfExp):
            self._validate_lambda_body(node.test, param_names)
            self._validate_lambda_body(node.body, param_names)
            self._validate_lambda_body(node.orelse, param_names)
            return
        if isinstance(node, ast.Compare):
            self._validate_lambda_body(node.left, param_names)
            for op in node.ops:
                if not isinstance(op, self.WHITE_LIST_LAMBDA_COMPARE_OPS):
                    self._reject("发现非法语法使用:[Lambda比较]，请修改")
            for comparator in node.comparators:
                self._validate_lambda_body(comparator, param_names)
            return
        if isinstance(node, ast.BinOp):
            if not isinstance(node.op, self.WHITE_LIST_LAMBDA_BINOPS):
                self._reject("发现非法语法使用:[Lambda运算]，请修改")
            self._validate_lambda_body(node.left, param_names)
            self._validate_lambda_body(node.right, param_names)
            return
        if isinstance(node, ast.Call):
            if isinstance(node.func, ast.Name) and node.func.id in self.WHITE_LIST_LAMBDA_FUNCTIONS:
                for arg in node.args:
                    self._validate_lambda_body(arg, param_names)
                for keyword in node.keywords:
                    if keyword.arg is None:
                        self._reject("发现非法语法使用:[Lambda参数]，请修改")
                    self._validate_lambda_body(keyword.value, param_names)
                return
            if isinstance(node.func, ast.Attribute) and node.func.attr in self.WHITE_LIST_LAMBDA_METHODS:
                self._validate_lambda_body(node.func.value, param_names)
                for arg in node.args:
                    self._validate_lambda_body(arg, param_names)
                for keyword in node.keywords:
                    if keyword.arg is None:
                        self._reject("发现非法语法使用:[Lambda参数]，请修改")
                    self._validate_lambda_body(keyword.value, param_names)
                return
        self._reject("发现非法语法使用:[Lambda表达式]，请修改")

    def generic_visit(self, node):
        if isinstance(node, self.FORBIDDEN_NODE_TYPES):
            self._reject("发现非法语法使用:[{}]，请修改".format(node.__class__.__name__))
        super().generic_visit(node)

    def visit_Module(self, node):
        template_functions = set()
        module_bindings = {}
        import_bindings = {}
        allow_top_level_helpers = True
        if allow_top_level_helpers:
            for stmt in node.body:
                if isinstance(stmt, ast.FunctionDef):
                    self._validate_binding_name(stmt.name)
                    self._remove_binding_names(
                        {stmt.name}, module_bindings, import_bindings, template_functions
                    )
                    template_functions.add(stmt.name)
                    continue
                if isinstance(stmt, ast.Import):
                    for name in stmt.names:
                        binding_name, module_name = self._module_import_binding(name)
                        self._remove_binding_names(
                            {binding_name}, module_bindings, import_bindings, template_functions
                        )
                        module_bindings[binding_name] = module_name
                    continue
                if isinstance(stmt, ast.ImportFrom):
                    module_name = self._import_from_module_name(stmt)
                    for name in stmt.names:
                        binding_name, member_path = self._from_import_binding(module_name, name)
                        self._remove_binding_names(
                            {binding_name}, module_bindings, import_bindings, template_functions
                        )
                        import_bindings[binding_name] = (module_name, member_path)
                    continue
                self._remove_binding_names(
                    self._statement_binding_names(stmt), module_bindings, import_bindings, template_functions
                )

        self._module_template_function_stack.append(template_functions)
        self._module_allowed_module_binding_stack.append(module_bindings)
        self._module_allowed_import_binding_stack.append(import_bindings)
        try:
            for stmt in node.body:
                self._allow_template_function_def_stack.append(
                    allow_top_level_helpers and isinstance(stmt, ast.FunctionDef)
                )
                try:
                    self.visit(stmt)
                finally:
                    self._allow_template_function_def_stack.pop()
        finally:
            self._module_allowed_import_binding_stack.pop()
            self._module_allowed_module_binding_stack.pop()
            self._module_template_function_stack.pop()

    def visit_Attribute(self, node):
        """访问属性节点"""
        if self._is_dunder(node.attr):
            raise ForbiddenMakoTemplateException("发现非法属性使用:[{}]，请修改".format(node.attr))

        module_name, member_path = self._module_member_path(node)
        if module_name:
            if self._is_allowed_module_attr_path(module_name, member_path):
                return
            self._reject("发现非法属性使用:[{}]，请修改".format(".".join(member_path)))

        if isinstance(node.value, ast.Name) and node.value.id == "this":
            return

        if node.attr in self.WHITE_LIST_ATTRS or node.attr in self.WHITE_LIST_METHODS:
            self.visit(node.value)
            return

        self._reject("发现非法属性使用:[{}]，请修改".format(node.attr))

    def visit_Call(self, node):
        """访问函数调用节点"""
        func = node.func
        if isinstance(func, ast.Name):
            if func.id in self.allowed_import_bindings:
                module_name, member_path = self.allowed_import_bindings[func.id]
                if not self._is_allowed_module_call_path(module_name, member_path):
                    self._reject("发现非法函数调用:[{}]，请修改".format(func.id))
            elif func.id in self.allowed_template_functions:
                self._validate_template_function_current_bindings(func.id)
            elif func.id in self.allowed_lambda_bindings:
                pass
            elif func.id in self.WHITE_LIST_EXCEPTIONS:
                pass
            elif func.id not in self.WHITE_LIST_FUNCTIONS:
                self._reject("发现非法函数调用:[{}]，请修改".format(func.id))
        elif isinstance(func, ast.Attribute):
            if self._is_dunder(func.attr):
                self._reject("发现非法函数调用:[{}]，请修改".format(func.attr))
            module_name, member_path = self._module_member_path(func)
            if module_name:
                if not self._is_allowed_module_call_path(module_name, member_path):
                    self._reject("发现非法函数调用:[{}]，请修改".format(".".join(member_path)))
                if module_name == "random" and member_path == ("shuffle",):
                    self._validate_random_shuffle_call(node)
                    return
            elif func.attr not in self.WHITE_LIST_METHODS:
                self._reject("发现非法函数调用:[{}]，请修改".format(func.attr))
            elif func.attr in ("setdefault", "update"):
                self._validate_dict_mutation_receiver(func.value)
        else:
            self._reject("发现非法函数调用:[{}]，请修改".format(func.__class__.__name__))
        self.generic_visit(node)

    def visit_FunctionDef(self, node):
        """访问顶层模板 helper 函数定义（禁止嵌套 def 与装饰器）"""
        if node.decorator_list:
            self._reject("发现非法语法使用:[带装饰器的函数定义]，请修改")
        if self._function_def_depth > 0 or not self._can_publish_template_function():
            self._reject("发现非法语法使用:[{}]，请修改".format(node.__class__.__name__))
        self._validate_binding_name(node.name)
        self._visit_function_annotations(node)
        local_binding_names = self._collect_function_local_bindings(node)

        self._function_def_depth += 1
        self._remove_allowed_bindings({node.name})
        self.allowed_template_functions.add(node.name)
        try:
            for default in node.args.defaults:
                self.visit(default)
            for default in node.args.kw_defaults:
                if default is not None:
                    self.visit(default)

            outer_module_bindings = self.allowed_module_bindings
            outer_import_bindings = self.allowed_import_bindings
            outer_template_functions = self.allowed_template_functions
            outer_lambda_bindings = self.allowed_lambda_bindings
            self.allowed_module_bindings = dict(outer_module_bindings)
            self.allowed_module_bindings.update(self._current_module_allowed_module_bindings())
            self.allowed_import_bindings = dict(outer_import_bindings)
            self.allowed_import_bindings.update(self._current_module_allowed_import_bindings())
            self.allowed_template_functions = (
                set(outer_template_functions) | self._current_module_template_functions()
            )
            self.allowed_lambda_bindings = set(outer_lambda_bindings)
            self._remove_allowed_bindings(local_binding_names)
            try:
                for stmt in node.body:
                    self.visit(stmt)
            finally:
                self.allowed_module_bindings = outer_module_bindings
                self.allowed_import_bindings = outer_import_bindings
                self.allowed_template_functions = outer_template_functions
                self.allowed_lambda_bindings = outer_lambda_bindings
            self._template_function_defs[node.name] = node
        finally:
            self._function_def_depth -= 1

    def visit_Try(self, node):
        """允许 try/except，异常处理块仍受常规白名单约束。"""
        for stmt in node.body:
            self.visit(stmt)
        for handler in node.handlers:
            if handler.type is not None:
                self.visit(handler.type)
            for stmt in handler.body:
                self.visit(stmt)
        for stmt in node.orelse:
            self.visit(stmt)
        for stmt in node.finalbody:
            self.visit(stmt)

    def visit_Raise(self, node):
        """仅允许 raise Exception(...) / raise ValueError(...)。"""
        if node.exc is not None:
            self._validate_raise_exc(node.exc)
        if node.cause is not None:
            self.visit(node.cause)

    def visit_Lambda(self, node):
        """仅允许单参数、无函数调用的简单 key 函数（如 sorted(..., key=lambda i: i['id'])）。"""
        if (
            len(node.args.args) > 1
            or node.args.vararg
            or node.args.kwarg
            or node.args.kwonlyargs
            or node.args.posonlyargs
            or node.args.kw_defaults
        ):
            self._reject("发现非法语法使用:[Lambda参数]，请修改")
        for default in node.args.defaults:
            self.visit(default)
        param_names = {arg.arg for arg in node.args.args}
        self._validate_lambda_body(node.body, param_names)

    def visit_ListComp(self, node):
        """允许单层列表推导式，禁止嵌套推导。"""
        if self._comprehension_depth > 0:
            self._reject("发现非法语法使用:[嵌套ListComp]，请修改")
        self._comprehension_depth += 1
        try:
            for generator in node.generators:
                self.visit(generator.iter)
                self._unbind_target(generator.target)
                for if_node in generator.ifs:
                    self.visit(if_node)
            self.visit(node.elt)
        finally:
            self._comprehension_depth -= 1

    def visit_GeneratorExp(self, node):
        """允许单层生成器表达式，规则与 ListComp 一致。"""
        if self._comprehension_depth > 0:
            self._reject("发现非法语法使用:[嵌套GeneratorExp]，请修改")
        self._comprehension_depth += 1
        try:
            for generator in node.generators:
                self.visit(generator.iter)
                self._unbind_target(generator.target)
                for if_node in generator.ifs:
                    self.visit(if_node)
            self.visit(node.elt)
        finally:
            self._comprehension_depth -= 1

    def visit_Assign(self, node):
        """访问赋值节点"""
        self.visit(node.value)
        callable_binding = self._allowed_callable_import_binding(node.value)
        for target in node.targets:
            self._visit_assign_target(target)
            self._bind_callable_alias_target(target, callable_binding)
            self._bind_lambda_target(target, node.value)

    def visit_AnnAssign(self, node):
        """访问带类型标注的赋值节点"""
        if node.annotation is not None:
            self.visit(node.annotation)
        callable_binding = None
        if node.value is not None:
            self.visit(node.value)
            callable_binding = self._allowed_callable_import_binding(node.value)
        self._visit_assign_target(node.target)
        self._bind_callable_alias_target(node.target, callable_binding)
        self._bind_lambda_target(node.target, node.value)

    def visit_AugAssign(self, node):
        """访问复合赋值节点"""
        self._visit_assign_target(node.target)
        self.visit(node.value)

    def visit_For(self, node):
        """访问循环节点"""
        self.visit(node.iter)
        self._unbind_target(node.target)
        for child in node.body:
            self.visit(child)
        for child in node.orelse:
            self.visit(child)

    def visit_Name(self, node):
        """访问名称节点"""
        self._validate_binding_name(node.id)

    def visit_Import(self, node):
        """访问导入节点"""
        for name in node.names:
            binding_name, module_name = self._module_import_binding(name)
            self._remove_allowed_bindings({binding_name})
            self.allowed_module_bindings[binding_name] = module_name

    def visit_ImportFrom(self, node):
        """访问从模块导入节点"""
        module_name = self._import_from_module_name(node)
        for name in node.names:
            binding_name, member_path = self._from_import_binding(module_name, name)
            self._remove_allowed_bindings({binding_name})
            self.allowed_import_bindings[binding_name] = (module_name, member_path)
