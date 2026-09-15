package setrixdb_test

import (
	"fmt"

	"github.com/setrixdb/setrixdb"
)

func ExampleIntersect() {
	// "Quais produtos são vermelhos E tamanho M?"
	vermelho := setrixdb.NewSet(1, 2, 3, 4, 5)
	tamM := setrixdb.NewSet(3, 4, 6)

	resultado := setrixdb.Intersect(vermelho, tamM)
	fmt.Println(resultado.Len(), resultado.Has(3), resultado.Has(1))

	// Output: 2 true false
}
