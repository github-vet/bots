package main

func main() {
	var y B
	IsPodReady(&y)
}

type A struct {
	x *int
}

func (a *A) twoRet() (*int, int) {
	x := 1
	return &x, 2
}

func (a *A) foo3(x, y *int, z int) *int {
	y = x
	return x
}

func bar(x, y *int) A {
	unsafe(y)
	return A{x}
}

var z *int

func unsafe(x *int) *int {
	z = x
	return z
}

func safe(a *A) int {
	y, _ := a.twoRet()
	return *y
}

type B struct {
	Status int
}

func IsPodReady(pod *B) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

func IsPodReadyConditionTrue(status int) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && *condition == ""
}
func GetPodReadyCondition(status int) *string {
	_, condition := GetPodCondition(&status, "PodReady")
	return condition
}

func GetPodCondition(status *int, conditiionType string) (int, *string) {
	x := ""
	return -1, &x
}
