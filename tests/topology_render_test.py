import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).parents[1] / "src/topology.star"

def _starlark_type(value):
    if value is None:
        return "NoneType"
    if isinstance(value, bool):
        return "bool"
    if isinstance(value, int):
        return "int"
    if isinstance(value, float):
        return "float"
    if isinstance(value, str):
        return "string"
    if isinstance(value, list):
        return "list"
    if isinstance(value, dict):
        return "dict"
    return "other"


def _load_topology():
    namespace = {
        "fail": lambda message: (_ for _ in ()).throw(ValueError(message)),
        "struct": lambda **kwargs: kwargs,
        "type": _starlark_type,
    }
    exec(MODULE_PATH.read_text(), namespace)
    return namespace


topology_namespace = _load_topology()


class TopologyRenderingTest(unittest.TestCase):
    def test_default_upvotes_drop_implicitly_vetoed_amendment(self):
        amendment_id, amendment_name = topology_namespace["GENESIS_AMENDMENTS"][0]
        settings = topology_namespace["_normalise_network_config"](
            {"veto_amendments": [{"id": amendment_id, "name": amendment_name}]}
        )
        ids = [amendment["id"] for amendment in settings["amendments"]]
        self.assertNotIn(amendment_id, ids)
        self.assertEqual(settings["veto_amendments"][0]["id"], amendment_id)

    def test_explicit_upvote_and_veto_overlap_fails(self):
        amendment_id, amendment_name = topology_namespace["GENESIS_AMENDMENTS"][0]
        with self.assertRaisesRegex(ValueError, "both amendments and veto_amendments"):
            topology_namespace["_normalise_network_config"](
                {
                    "amendments": [{"id": amendment_id, "name": amendment_name}],
                    "veto_amendments": [{"id": amendment_id, "name": amendment_name}],
                }
            )

    def test_template_injects_config_as_data(self):
        class Plan:
            def render_templates(self, **kwargs):
                return kwargs

        artifact = topology_namespace["generate_network_config"](
            Plan(),
            2,
            0,
            rippled_config={"custom": ["value={{literal}}"]},
        )
        entry = artifact["config"]["rippled-0.cfg"]
        self.assertEqual(entry["template"], "{{.config}}")
        self.assertIn("value={{literal}}", entry["data"]["config"])


if __name__ == "__main__":
    unittest.main()
