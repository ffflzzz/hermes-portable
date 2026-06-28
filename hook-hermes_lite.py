# Hook file for hermes_lite - tells PyInstaller to include all needed modules
"""
Hook file for hermes_lite to ensure PyInstaller includes all referenced modules.
Place in same directory as hermes_lite.py or use --additional-hooks-dir.
"""

from PyInstaller.utils.hooks import collect_all

# Collect all submodules and data files
datas = []
binaries = []

# Explicitly collect these modules that PyInstaller might miss
hiddenimports = [
    'requests',
    'subprocess',
    'json',
    'os',
    'sys',
    'platform',
    'socket',
    'threading',
    'time',
    'traceback',
]

# Add all functions as references to prevent stripping
def _keep_alive():
    # These references prevent PyInstaller from stripping the functions
    import hermes_lite
    _ = hermes_lite.tool_terminal
    _ = hermes_lite.tool_read_file
    _ = hermes_lite.tool_write_file
    _ = hermes_lite.tool_search_files
    _ = hermes_lite.tool_patch_file
    _ = hermes_lite.TOOLS
    _ = hermes_lite.TOOL_HANDLERS
    _ = hermes_lite.call_api
    _ = hermes_lite.SYSTEM_PROMPT
