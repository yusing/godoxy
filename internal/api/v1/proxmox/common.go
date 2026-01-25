package proxmoxapi

type ActionRequest struct {
	Node string `uri:"node" binding:"required"`
	VMID int    `uri:"vmid" binding:"required"`
}
