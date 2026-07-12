# /// script
# dependencies = [
#   "mcp",
#   "requests",
#   "pydantic"
# ]
# ///
import os
from typing import Optional, List, Dict, Any
import requests
from mcp.server.fastmcp import FastMCP

# Initialize FastMCP server
mcp = FastMCP("ICS-Core")

# Configuration
ICS_BASE_URL = os.environ.get("ICS_BASE_URL", "http://127.0.0.1:8666")
ICS_TOKEN = os.environ.get("ICS_TOKEN", "")

def get_headers() -> Dict[str, str]:
    return {"X-Auth-Token": ICS_TOKEN}

@mcp.tool()
def list_messages(channel: str = "") -> List[Dict[str, Any]]:
    """
    List all current messages and files in the ICS-Core stream.
    Returns a list of items (text or file cards) ordered by newest first.
    
    Args:
        channel: Optional channel name to fetch messages from a specific namespace.
    """
    params = {"channel": channel} if channel else {}
    resp = requests.get(f"{ICS_BASE_URL}/api/list", headers=get_headers(), params=params, timeout=30)
    resp.raise_for_status()
    return resp.json()

@mcp.tool()
def read_message(item_id: str) -> str:
    """
    Read the full raw content of a specific message or file.
    Use this to pull the exact content of a newly triggered message without listing the entire channel.
    
    Args:
        item_id: The unique ID of the message/file (e.g. 'upwork/1780183356_pc_text.txt')
    """
    # The /api/download/ endpoint serves the raw file contents
    resp = requests.get(f"{ICS_BASE_URL}/api/download/{item_id}", headers=get_headers(), timeout=30)
    resp.raise_for_status()
    return resp.text

@mcp.tool()
def push_message(text: str, device: str = "ai", channel: str = "") -> Dict[str, Any]:
    """
    Push a new text message to the ICS-Core stream.
    
    Args:
        text: The text content to push.
        device: The device identifier (default is 'ai', can also be 'pc', 'mobile', etc.)
        channel: Optional channel name to push the message to a specific namespace.
    """
    # Send as multipart/form-data which is required by some endpoints
    # By using `files` with (None, value), requests formats it as multipart.
    multipart_data = {
        "text": (None, text),
        "device": (None, device)
    }
    params = {"channel": channel} if channel else {}
    resp = requests.post(f"{ICS_BASE_URL}/api/push", headers=get_headers(), params=params, files=multipart_data, timeout=30)
    resp.raise_for_status()
    return resp.json()

@mcp.tool()
def push_file(file_path: str, device: str = "ai", channel: str = "") -> Dict[str, Any]:
    """
    Push a local file to the ICS-Core stream. The file is uploaded as-is with its original filename.

    Args:
        file_path: Absolute path to the local file to upload (e.g. 'C:/Users/you/report.pdf').
        device: The device identifier (default is 'ai', can also be 'pc', 'mobile', etc.)
        channel: Optional channel name to push the file to a specific namespace.
    """
    import os as _os
    if not _os.path.isfile(file_path):
        raise FileNotFoundError(f"File not found: {file_path}")
    filename = _os.path.basename(file_path)
    params = {"channel": channel} if channel else {}
    with open(file_path, "rb") as f:
        multipart_data = {
            "file": (filename, f),
            "device": (None, device),
        }
        resp = requests.post(f"{ICS_BASE_URL}/api/push", headers=get_headers(), params=params, files=multipart_data, timeout=30)
    resp.raise_for_status()
    return resp.json()

@mcp.tool()
def manage_message(item_id: str, action: str) -> Dict[str, Any]:
    """
    Manage an existing message in the ICS-Core stream.
    
    Args:
        item_id: The unique ID of the message/file item.
        action: The action to perform. Must be one of: 'pin', 'unpin', 'delete'.
    """
    if action not in ("pin", "unpin", "delete"):
        raise ValueError("action must be one of: 'pin', 'unpin', 'delete'")
        
    payload = {
        "id": item_id,
        "action": action
    }
    resp = requests.post(f"{ICS_BASE_URL}/api/action", headers=get_headers(), json=payload, timeout=30)
    resp.raise_for_status()
    return resp.json()

if __name__ == "__main__":
    mcp.run()
