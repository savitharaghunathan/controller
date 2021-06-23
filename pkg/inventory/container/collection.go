/*
A collection of models.
Provides methods to reconcile the collection of stored models
with the collection of desired models.
*/
package container

import (
	fb "github.com/konveyor/controller/pkg/filebacked"
	"github.com/konveyor/controller/pkg/inventory/model"
	"reflect"
)

//
// Model shepherd.
type Shepherd interface {
	// Determine if model needs to be updated.
	Equals(mA, mB model.Model) bool
	// Update the stored model as desired.
	Update(stored, desired model.Model)
}

//
// Disposition.
type Disposition struct {
	// The stored models in the collection.
	stored model.Model
	// The desired models in the collection.
	desired model.Model
}

//
// Disposition map.
type Dispositions map[string]*Disposition

//
// Model collection.
type Collection struct {
	// Stored models.
	Stored fb.Iterator
	// DB transaction.
	Tx *model.Tx
	// An (optional) shepherd.
	Shepherd Shepherd
	// Number of models added.
	Added int
	// Number models updated.
	Updated int
	// Number models deleted.
	Deleted int
}

//
// Add models included in desired but not stored.
func (r *Collection) Add(desired fb.Iterator) error {
	return r.add(r.dispositions(desired))
}

//
// Update models.
func (r *Collection) Update(desired fb.Iterator) error {
	return r.update(r.dispositions(desired))
}

//
// Delete stored models not included in the desired.
func (r *Collection) Delete(desired fb.Iterator) error {
	return r.delete(r.dispositions(desired))
}

//
// Reconcile the collection.
// Ensure the stored collection is as desired.
func (r *Collection) Reconcile(desired fb.Iterator) (err error) {
	mp := r.dispositions(desired)
	err = r.delete(mp)
	if err != nil {
		return
	}
	err = r.add(mp)
	if err != nil {
		return
	}
	err = r.update(mp)
	if err != nil {
		return
	}

	return
}

//
// Build the dispositions.
func (r *Collection) dispositions(desired fb.Iterator) (mp map[string]*Disposition) {
	mp = map[string]*Disposition{}
	for object, hasNext := r.Stored.Next(); hasNext; object, hasNext = r.Stored.Next() {
		m := object.(model.Model)
		mp[m.Pk()] = &Disposition{
			stored: m,
		}
	}
	for object, hasNext := desired.Next(); hasNext; object, hasNext = desired.Next() {
		m := object.(model.Model)
		if dpn, found := mp[m.Pk()]; !found {
			mp[m.Pk()] = &Disposition{
				desired: m,
			}
		} else {
			dpn.desired = m
		}
	}

	return
}

//
// Add models included in desired but not stored.
func (r *Collection) add(dispositions Dispositions) (err error) {
	for _, dpn := range dispositions {
		if dpn.desired != nil && dpn.stored == nil {
			err = r.Tx.Insert(dpn.desired)
			if err == nil {
				r.Added++
			} else {
				return
			}
		}
	}

	return
}

//
// Update models.
func (r *Collection) update(dispositions Dispositions) (err error) {
	shepherd := r.Shepherd
	if shepherd == nil {
		shepherd = &DefaultShepherd{}
	}
	for _, dpn := range dispositions {
		if dpn.desired == nil || dpn.stored == nil {
			continue
		}
		if shepherd.Equals(dpn.desired, dpn.stored) {
			continue
		}
		shepherd.Update(dpn.stored, dpn.desired)
		err = r.Tx.Update(dpn.stored)
		if err == nil {
			r.Updated++
		} else {
			return
		}
	}

	return
}

//
// Delete stored models not included in the desired.
func (r *Collection) delete(dispositions Dispositions) (err error) {
	for _, dpn := range dispositions {
		if dpn.stored != nil && dpn.desired == nil {
			err = r.Tx.Delete(dpn.stored)
			if err == nil {
				r.Deleted++
			} else {
				return
			}
		}
	}

	return
}

//
// Default (reflect-based) shepherd.
// Fields are ignored when:
//   - Is the PK.
//   - Is (auto) incremented.
//   - Has the `eq:"-"` tag.
type DefaultShepherd struct {
}

//
// Model comparison.
func (r *DefaultShepherd) Equals(mA, mB model.Model) bool {
	fieldsA, _ := model.Table{}.Fields(mA)
	fieldsB, _ := model.Table{}.Fields(mB)
	for i := 0; i < len(fieldsA); i++ {
		fA := fieldsA[i]
		fB := fieldsB[i]
		if r.ignored(fA) {
			continue
		}
		vA := fA.Value.Interface()
		vB := fB.Value.Interface()
		if !reflect.DeepEqual(vA, vB) {
			return false
		}
	}

	return true
}

//
// Update model A (stored) with model B (desired).
func (r *DefaultShepherd) Update(mA, mB model.Model) {
	fieldsA, _ := model.Table{}.Fields(mA)
	fieldsB, _ := model.Table{}.Fields(mB)
	for i := 0; i < len(fieldsA); i++ {
		fA := fieldsA[i]
		fB := fieldsB[i]
		if r.ignored(fA) {
			continue
		}
		vB := fB.Value.Interface()
		fA.Value.Set(reflect.ValueOf(vB))
	}
}

//
// The field is ignored when:
//   - Is the PK.
//   - Is (auto) incremented.
//   - Has the `eq:"-"` tag.
func (r *DefaultShepherd) ignored(f *model.Field) bool {
	if f.Pk() || f.Incremented() {
		return true
	}
	if tag, found := f.Type.Tag.Lookup("eq"); found {
		if tag == "-" {
			return true
		}
	}

	return false
}
