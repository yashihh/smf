package smf_context

import "fmt"

type DataPathNode struct {
	UPF          *UPF
	DataPathToAN *DataPathDownLink
	DataPathToDN map[string]*DataPathUpLink //uuid to DataPathLink

	//for UE Routing Topology
	//for special case:
	//branching & leafnode
	IsBranchingPoint     bool
	DLDataPathLinkForPSA *DataPathUpLink
}

type DataPathDownLink struct {
	To *DataPathNode

	// Filter Rules
	DestinationIP   string
	DestinationPort string

	// related context
	UpLinkPDR *PDR
}

type DataPathUpLink struct {
	To *DataPathNode

	// Filter Rules
	DestinationIP   string
	DestinationPort string

	// related context
	DownLinkPDR *PDR
}

func NewDataPathNode() (node *DataPathNode) {

	node = &DataPathNode{
		UPF:                  nil,
		DataPathToDN:         make(map[string]*DataPathUpLink),
		DataPathToAN:         nil,
		IsBranchingPoint:     false,
		DLDataPathLinkForPSA: nil,
	}
	return
}

func NewDataPathDownLink() (link *DataPathDownLink) {

	link = &DataPathDownLink{
		To:              nil,
		DestinationIP:   "",
		DestinationPort: "",
		UpLinkPDR:       nil,
	}
	return
}

func NewDataPathUpLink() (link *DataPathUpLink) {

	link = &DataPathUpLink{
		To:              nil,
		DestinationIP:   "",
		DestinationPort: "",
		DownLinkPDR:     nil,
	}
	return
}

func (node *DataPathNode) AddChild(child *DataPathNode) (err error) {

	child_id, err := child.GetUPFID()

	if err != nil {
		return err
	}

	if _, exist := node.DataPathToDN[child_id]; !exist {

		child_link := &DataPathUpLink{
			To:              child,
			DestinationIP:   "",
			DestinationPort: "",
			DownLinkPDR:     nil,
		}
		node.DataPathToDN[child_id] = child_link
	}

	return
}

func (node *DataPathNode) AddParent(parent *DataPathNode) (err error) {

	parent_ip := parent.GetNodeIP()
	var exist bool

	if _, exist = smfContext.UserPlaneInformation.UPFsIPtoID[parent_ip]; !exist {
		err = fmt.Errorf("UPNode IP %s doesn't exist in smfcfg.conf, please sync the config files!", parent_ip)
		return err
	}

	if node.DataPathToAN == nil {

		parent_link := &DataPathDownLink{
			To:              parent,
			DestinationIP:   "",
			DestinationPort: "",
			UpLinkPDR:       nil,
		}

		node.DataPathToAN = parent_link
	}

	return
}

func (node *DataPathNode) AddDestinationOfChild(child *DataPathNode, Dest *DataPathUpLink) (err error) {

	child_id, err := child.GetUPFID()

	if err != nil {
		return err
	}
	if child_link, exist := node.DataPathToDN[child_id]; exist {

		child_link.DestinationIP = Dest.DestinationIP
		child_link.DestinationPort = Dest.DestinationPort

	}

	return
}

func (node *DataPathNode) GetUPFID() (id string, err error) {
	node_ip := node.GetNodeIP()
	var exist bool

	if id, exist = smfContext.UserPlaneInformation.UPFsIPtoID[node_ip]; !exist {
		err = fmt.Errorf("UPNode IP %s doesn't exist in smfcfg.conf, please sync the config files!", node_ip)
		return "", err
	}

	return id, nil

}

func (node *DataPathNode) GetNodeIP() (ip string) {

	ip = node.UPF.NodeID.ResolveNodeIdToIp().String()
	return
}

func (node *DataPathNode) PrintPath() (str string) {
	upi := smfContext.UserPlaneInformation
	str = upi.GetUPFNameByIp(node.GetNodeIP()) + "\n"

	for _, node := range node.DataPathToDN {
		str += node.To.PrintPath()
	}
	return
}

func (node *DataPathNode) IsANUPF() bool {

	if node.DataPathToAN.To == nil {
		return true
	} else {
		return false
	}
}

func (node *DataPathNode) IsAnchorUPF() bool {

	if len(node.DataPathToDN) == 0 {
		return true
	} else {
		return false
	}

}

func (node *DataPathNode) GetUpLinkPDR() (pdr *PDR) {
	return node.DataPathToAN.UpLinkPDR
}

func (node *DataPathNode) GetUpLinkFAR() (far *FAR) {
	return node.DataPathToAN.UpLinkPDR.FAR
}

func (node *DataPathNode) GetParent() (parent *DataPathNode) {
	return node.DataPathToAN.To
}
