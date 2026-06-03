# -*- coding: utf-8 -*-

import json
import subprocess
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace

PYTHON_ROOT = Path(__file__).resolve().parents[1]
if str(PYTHON_ROOT) not in sys.path:
    sys.path.insert(0, str(PYTHON_ROOT))

from mako_render import mako_render
from mako_render.checker import check_mako_template_safety
from mako_render.exceptions import ForbiddenMakoTemplateException


class MakoSafetyTest(unittest.TestCase):
    def assert_unsafe(self, template):
        with self.assertRaises(ForbiddenMakoTemplateException):
            check_mako_template_safety(template)

    def test_rejects_unsafe_template_features(self):
        cases = [
            '${__import__("os").system("id")}',
            '${().__class__.__mro__[1].__subclasses__()}',
            '${sorted([2, 1])}',
            '${getattr(this, "cc_host", None)}',
            '${open("/etc/passwd").read()}',
            '${open("/etc/passwd").read().replace("a", "b")}',
            '${__import__("os").system("id").replace("x", "y")}',
            '${json.system("id")}',
        ]

        for template in cases:
            with self.subTest(template=template):
                self.assert_unsafe(template)

    def test_rejects_calls_on_rebound_allowed_module_names(self):
        cases = [
            """<%
import json
json = attacker
%>
${json.system("id")}""",
            """<%
import json as safe_json
safe_json = attacker
%>
${safe_json.system("id")}""",
        ]

        for template in cases:
            with self.subTest(template=template):
                self.assert_unsafe(template)

    def test_rejects_unsafe_expression_filters(self):
        cases = [
            '${"open(\\"/etc/hosts\\").read()" | eval}',
            '${"__import__(\\"os\\").system(\\"id\\")" | eval}',
            '${"/etc/hosts" | open}',
            '<%text filter="eval">1+1</%text>',
        ]

        for template in cases:
            with self.subTest(template=template):
                self.assert_unsafe(template)

    def test_allows_safe_expression_filters(self):
        result = mako_render('${"  <bscp>  " | h,trim}', {})

        self.assertEqual("&lt;bscp&gt;", result)

    def test_allows_assignment_rhs_to_use_current_module_binding(self):
        template = """<%
import json
json = json.dumps({"name": "bscp"})
%>
${json.replace("bscp", "BSCP")}"""

        result = mako_render(template, {})

        self.assertIn('"name": "BSCP"', result)

    def test_rejects_module_calls_after_assignment_rebinds_name(self):
        template = """<%
import json
json = json.dumps({"name": "bscp"})
%>
${json.dumps({"name": "bscp"})}"""

        self.assert_unsafe(template)

    def test_allows_for_iter_to_use_current_module_binding(self):
        template = """<%
import json
%>
% for json in [json.dumps({"name": "bscp"})]:
${json.replace("bscp", "BSCP")}
% endfor"""

        result = mako_render(template, {})

        self.assertIn('"name": "BSCP"', result)

    def test_rejects_module_calls_after_for_target_rebinds_name(self):
        template = """<%
import json
%>
% for json in [json.dumps({"name": "bscp"})]:
${json.dumps({"name": "bscp"})}
% endfor"""

        self.assert_unsafe(template)

    @unittest.skipIf(sys.version_info < (3, 10), "pattern matching requires Python 3.10+")
    def test_rejects_pattern_matching_rebound_allowed_module_names(self):
        template = """<%
import json
match attacker:
    case _ as json:
        pass
%>
${json.dumps("id")}"""

        self.assert_unsafe(template)

    def test_rejects_callable_context_values(self):
        context = {
            "obj": SimpleNamespace(get=lambda _: "unsafe"),
        }

        with self.assertRaises(ForbiddenMakoTemplateException):
            mako_render('${obj.get("id")}', context)

    def test_rejects_cc_xml_with_doctype(self):
        cc_xml = '<!DOCTYPE root [<!ENTITY xxe "INTERNAL">]><root>&xxe;</root>'
        process = subprocess.run(
            [sys.executable, str(PYTHON_ROOT / "main.py"), "--stdin"],
            input=json.dumps({"template": "ok", "context": {"cc_xml": cc_xml}}),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

        self.assertNotEqual(0, process.returncode)
        self.assertIn("DOCTYPE", process.stderr)

    def test_allows_normal_cc_xml_context(self):
        cc_xml = (
            '<?xml version="1.0" encoding="UTF-8"?>'
            '<Application><Set SetName="qq"><Module ModuleName="gamesvr">'
            '<Host InnerIP="127.0.0.1" bk_cloud_id="0" />'
            '</Module></Set></Application>'
        )
        process = subprocess.run(
            [sys.executable, str(PYTHON_ROOT / "main.py"), "--stdin"],
            input=json.dumps(
                {
                    "template": '${this.cc_host.attrib.get("InnerIP")}',
                    "context": {
                        "cc_xml": cc_xml,
                        "bk_set_name": "qq",
                        "bk_module_name": "gamesvr",
                        "bk_host_innerip": "127.0.0.1",
                        "bk_cloud_id": "0",
                    },
                }
            ),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

        self.assertEqual("", process.stderr)
        self.assertEqual(0, process.returncode)
        self.assertEqual("127.0.0.1", process.stdout)

    def test_rejects_unapproved_allowed_module_members(self):
        cases = [
            """<%
import random
%>
${random._os.open("/etc/hosts", 0)}""",
            """<%
import random
%>
${random._os.read(0, 10)}""",
            """<%
import json
%>
${json.tool}""",
            """<%
from random import _os
%>
${_os.open("/etc/hosts", 0)}""",
            """<%
from json import tool
%>
${tool}""",
            """<%
import json.tool as json_tool
%>
${json_tool.dumps({"name": "bscp"})}""",
            """<%
from json.tool import dumps
%>
${dumps({"name": "bscp"})}""",
            """<%
import math
%>
${math.__dict__}""",
        ]

        for template in cases:
            with self.subTest(template=template):
                self.assert_unsafe(template)

    def test_allows_explicit_safe_module_members(self):
        template = """<%
import datetime
import json
import math
import random
import re
%>
${datetime.datetime.now()}
${json.dumps({"name": "bscp"})}
${math.sqrt(4)}
${random.randint(1, 1)}
${re.compile("bs")}
"""

        result = mako_render(template, {})

        self.assertIn('"name": "bscp"', result)
        self.assertIn("2.0", result)
        self.assertIn("1", result)
        self.assertIn("bs", result)

    def test_allows_safe_members_imported_from_allowed_modules(self):
        template = """<%
from math import sqrt
from json import dumps as to_json
%>
${sqrt(9)}
${to_json({"name": "bscp"})}
"""

        result = mako_render(template, {})

        self.assertIn("3.0", result)
        self.assertIn('"name": "bscp"', result)

    def test_allows_business_template_features(self):
        template = """Hello ${name}
${data.get("role", "none")}
${text.replace("a", "b")}
% for idx, item in enumerate(items):
${idx}:${item}
% endfor"""

        result = mako_render(
            template,
            {
                "name": "BSCP",
                "data": {"role": "server"},
                "text": "a-a",
                "items": ["x", "y"],
            },
        )

        self.assertIn("Hello BSCP", result)
        self.assertIn("server", result)
        self.assertIn("b-b", result)
        self.assertIn("0:x", result)
        self.assertIn("1:y", result)

    def test_allows_template_helper_functions(self):
        template = """<%
def getAppId():
    if this.cc_set.attrib['bk_set_name'] == "prod":
        return ""
    return ""

def getMongoInfo():
    if this.cc_set.attrib['bk_set_name'] == "prod":
        return "mongodb://g"
    return "mongodb://gameus"
%>
${getAppId()}|${getMongoInfo()}"""

        self.assertTrue(check_mako_template_safety(template))

        this = SimpleNamespace(
            cc_set=SimpleNamespace(attrib={"bk_set_name": "prod"}),
        )
        result = mako_render(template, {"this": this})

        self.assertIn("|mongodb://g", result)

    def test_rejects_nested_template_function_def(self):
        template = """<%
def outer():
    def inner():
        return 1
%>"""

        self.assert_unsafe(template)

    def test_rejects_decorated_template_function_def(self):
        template = """<%
def helper():
    return 1

@decorator
def wrapped():
    return 2
%>"""

        self.assert_unsafe(template)

    def test_rejects_unsafe_code_inside_template_function(self):
        template = """<%
def evil():
    return open("/etc/passwd").read()
%>"""

        self.assert_unsafe(template)

    def test_function_local_assignment_does_not_clear_outer_module_binding(self):
        template = """<%
import json

def helper():
    json = {}
    return "ok"
%>
${json.dumps({"name": "bscp"})}"""

        result = mako_render(template, {})

        self.assertIn('"name": "bscp"', result)

    def test_function_local_import_does_not_create_outer_module_binding(self):
        template = """<%
def helper():
    import json
    return json.dumps({"name": "bscp"})
%>
${json.dumps({"name": "bscp"})}"""

        self.assert_unsafe(template)

    def test_rejects_module_call_after_helper_rebinds_module_name(self):
        template = """<%
import json

def json():
    return "helper"
%>
${json.dumps({"name": "bscp"})}"""

        self.assert_unsafe(template)

    def test_allows_helper_to_call_later_helper_in_same_code_block(self):
        template = """<%
def first():
    return second()

def second():
    return "helper"
%>
${first()}"""

        result = mako_render(template, {})

        self.assertIn("helper", result)

    def test_allows_helper_to_use_later_module_import_in_same_code_block(self):
        template = """<%
def helper():
    return json.dumps({"name": "bscp"})

import json
%>
${helper()}"""

        result = mako_render(template, {})

        self.assertIn('"name": "bscp"', result)

    def test_allows_helper_to_use_later_from_import_in_same_code_block(self):
        template = """<%
def helper():
    return to_json({"name": "bscp"})

from json import dumps as to_json
%>
${helper()}"""

        result = mako_render(template, {})

        self.assertIn('"name": "bscp"', result)

    def test_rejects_helper_later_module_import_rebound_in_same_code_block(self):
        template = """<%
def helper():
    return json.dumps({"name": "bscp"})

import json
json = attacker
%>
${helper()}"""

        self.assert_unsafe(template)

    def test_rejects_helper_import_inside_python_control_block(self):
        template = """<%
def helper():
    return json.dumps({"name": "bscp"})

if flag:
    import json
%>
${helper()}"""

        self.assert_unsafe(template)

    def test_rejects_helper_call_to_rebound_later_helper_name(self):
        template = """<%
def first():
    return second()

def second():
    return "helper"

second = attacker
%>
${first()}"""

        self.assert_unsafe(template)

    def test_rejects_call_after_later_import_rebinds_helper_name(self):
        template = """<%
def json():
    return "helper"

import json
%>
${json()}"""

        self.assert_unsafe(template)

    def test_rejects_module_attr_after_from_import_rebinds_module_name(self):
        template = """<%
import json
from math import sqrt as json
%>
${json.dumps({"name": "bscp"})}"""

        self.assert_unsafe(template)

    def test_rejects_helper_defined_inside_mako_if_block(self):
        template = """% if flag:
<%
def helper():
    return "helper"
%>
% endif
${helper()}"""

        self.assert_unsafe(template)

    def test_rejects_helper_defined_inside_mako_for_block(self):
        template = """% for item in items:
<%
def helper():
    return "helper"
%>
% endfor
${helper()}"""

        self.assert_unsafe(template)

    def test_allows_import_defined_inside_mako_if_block_for_inner_nodes(self):
        template = """% if flag:
<%
import json
%>
${json.dumps({"name": "bscp"})}
% endif"""

        result = mako_render(template, {"flag": True})

        self.assertIn('"name": "bscp"', result)

    def test_rejects_import_defined_inside_mako_if_block_leaking_out(self):
        template = """% if flag:
<%
import json
%>
% endif
${json.dumps({"name": "bscp"})}"""

        self.assert_unsafe(template)

    def test_rejects_mako_control_rebind_of_outer_module_binding(self):
        template = """<%
import json
%>
% if flag:
<%
json = attacker
%>
% endif
${json.dumps({"name": "bscp"})}"""

        self.assert_unsafe(template)

    def test_rejects_helper_defined_inside_python_control_block(self):
        template = """<%
if flag:
    def helper():
        return "helper"
%>
${helper()}"""

        self.assert_unsafe(template)

    def test_rejects_unsafe_template_function_annotations(self):
        cases = [
            """<%
def helper(value: open("/etc/passwd").read()):
    return "ok"
%>""",
            """<%
def helper() -> open("/etc/passwd").read():
    return "ok"
%>""",
        ]

        for template in cases:
            with self.subTest(template=template):
                self.assert_unsafe(template)

    def test_rejects_call_before_template_function_def(self):
        template = """${getAppId()}
<%
def getAppId():
    return ""
%>"""

        self.assert_unsafe(template)

    def test_rejects_rebound_template_function_name(self):
        template = """<%
def getAppId():
    return "safe"
getAppId = attacker
%>
${getAppId()}"""

        self.assert_unsafe(template)

    def test_allows_help_template_safety_check(self):
        from main import HELP_TEMPLATE

        self.assertTrue(check_mako_template_safety(HELP_TEMPLATE))


if __name__ == "__main__":
    unittest.main()
