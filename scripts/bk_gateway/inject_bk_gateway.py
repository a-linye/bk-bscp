#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
import re
import sys


# 与蓝鲸网关 apigateway/apis/web/resource/constants.py 的 PATH_PATTERN 保持一致。
# 网关对前端路径和 backend_config.path 都用该正则校验，冒号等字符会导致导入报
# “校验失败: backend_config.path: 输入值不匹配要求的模式”。
GATEWAY_PATH_PATTERN = re.compile(r"^/[\w{}/.-]*$")

DEFAULT_EXTENSIONS = {
    "isPublic": True,
    "allowApplyPermission": True,
    "authConfig": {
        "appVerifiedRequired": True,
        "userVerifiedRequired": True,
        "resourcePermissionRequired": True
    }
}


def build_inner_extensions(method, path):
    """为 /inner/ 接口动态构建蓝鲸网关扩展配置，通过 backend.path 将网关路径映射到实际后端路径。"""
    backend_path = path.replace("/inner/", "/", 1)
    return {
        "isPublic": False,
        "allowApplyPermission": True,
        "backend": {
            "type": "HTTP",
            "method": method,
            "path": backend_path,
            "matchSubpath": False,
            "timeout": 0,
            "upstreams": {},
            "transformHeaders": {}
        },
        "authConfig": {
            "appVerifiedRequired": True,
            "userVerifiedRequired": False,
            "resourcePermissionRequired": True
        }
    }


def collect_path_errors(paths):
    """校验注入结果中的路径能否被蓝鲸网关接受。未显式配置 backend 时，网关以前端路径作为后端路径。"""
    errors = []
    for path, path_item in paths.items():
        if not GATEWAY_PATH_PATTERN.match(path):
            errors.append(f"前端请求路径不合法: {path}")
        for method, method_config in path_item.items():
            if not isinstance(method_config, dict):
                continue
            extensions = method_config.get("x-bk-apigateway-resource") or {}
            backend_path = (extensions.get("backend") or {}).get("path") or path
            if not GATEWAY_PATH_PATTERN.match(backend_path):
                errors.append(f"后端请求路径不合法: {method.upper()} {path} -> {backend_path}")
    return errors


def inject_bk_gateway_config(file_path):
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            swagger_data = json.load(f)
    except Exception as e:
        print(f"读取或解析文件失败 [{file_path}]: {e}")
        return False

    paths = swagger_data.get("paths", {})
    inner_count = 0
    default_count = 0

    for path, path_item in paths.items():
        is_inner = "/inner/" in path
        for method, method_config in path_item.items():
            if not isinstance(method_config, dict):
                continue
            if is_inner:
                method_config["x-bk-apigateway-resource"] = build_inner_extensions(method, path)
                inner_count += 1
            else:
                method_config.setdefault("x-bk-apigateway-resource", DEFAULT_EXTENSIONS)
                default_count += 1

    inject_count = inner_count + default_count
    if inject_count == 0:
        print(f"未在 {file_path} 中检测到接口，跳过注入。")
        return True

    errors = collect_path_errors(paths)
    if errors:
        print(f"校验失败, {file_path} 中存在 {len(errors)} 个网关不接受的路径, 已跳过写入:")
        for error in errors:
            print(f"  - {error}")
        print("请修改 proto 中 google.api.http 的路径（如 gRPC 自定义动词的冒号）后重新生成。")
        return False

    try:
        with open(file_path, 'w', encoding='utf-8') as f:
            json.dump(swagger_data, f, indent=2, ensure_ascii=False)
        print(f"成功, 已为 {file_path} 注入蓝鲸网关配置: inner={inner_count}, default={default_count}。")
    except Exception as e:
        print(f"写入文件失败 [{file_path}]: {e}")
        return False
    return True


if __name__ == "__main__":
    # 支持从命令行传入多个文件路径
    if len(sys.argv) < 2:
        print("使用方法: python3 inject_bk_gateway.py <file1.json> <file2.json> ...")
        sys.exit(1)

    if not all([inject_bk_gateway_config(target_file) for target_file in sys.argv[1:]]):
        sys.exit(1)