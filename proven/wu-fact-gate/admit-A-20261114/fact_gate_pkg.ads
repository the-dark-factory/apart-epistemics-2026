package Fact_Gate_Pkg with SPARK_Mode, Pure is
  -- PURPOSE: the fact gate: COMPLETE only when all eight required facts are supplied, else the first missing one.
  -- USAGE: call Complete (P1 .. P8) and First_Missing (P1 .. P8); pass True for a fact the calculation does not need.

  type Fact_Index is range 0 .. 8;

  function Complete (P1 : Boolean; P2 : Boolean; P3 : Boolean; P4 : Boolean; P5 : Boolean; P6 : Boolean; P7 : Boolean; P8 : Boolean) return Boolean is
    (P1 and then P2 and then P3 and then P4 and then P5 and then P6 and then P7 and then P8);

  function First_Missing (P1 : Boolean; P2 : Boolean; P3 : Boolean; P4 : Boolean; P5 : Boolean; P6 : Boolean; P7 : Boolean; P8 : Boolean) return Fact_Index is
    ((if not P1 then 1 elsif not P2 then 2 elsif not P3 then 3 elsif not P4 then 4 elsif not P5 then 5 elsif not P6 then 6 elsif not P7 then 7 elsif not P8 then 8 else 0));

  function Never_Complete_On_A_Gap (P1 : Boolean; P2 : Boolean; P3 : Boolean; P4 : Boolean; P5 : Boolean; P6 : Boolean; P7 : Boolean; P8 : Boolean) return Boolean is
    ((if not (P1 and then P2 and then P3 and then P4 and then P5 and then P6 and then P7 and then P8) then not Complete (P1, P2, P3, P4, P5, P6, P7, P8)))
    with Post => Never_Complete_On_A_Gap'Result;

  function Complete_When_Nothing_Missing (P1 : Boolean; P2 : Boolean; P3 : Boolean; P4 : Boolean; P5 : Boolean; P6 : Boolean; P7 : Boolean; P8 : Boolean) return Boolean is
    ((if P1 and then P2 and then P3 and then P4 and then P5 and then P6 and then P7 and then P8 then Complete (P1, P2, P3, P4, P5, P6, P7, P8)))
    with Post => Complete_When_Nothing_Missing'Result;

  function Zero_Exactly_When_Complete (P1 : Boolean; P2 : Boolean; P3 : Boolean; P4 : Boolean; P5 : Boolean; P6 : Boolean; P7 : Boolean; P8 : Boolean) return Boolean is
    (((First_Missing (P1, P2, P3, P4, P5, P6, P7, P8) = 0) = Complete (P1, P2, P3, P4, P5, P6, P7, P8)))
    with Post => Zero_Exactly_When_Complete'Result;

  function Named_Fact_Is_Really_Missing (P1 : Boolean; P2 : Boolean; P3 : Boolean; P4 : Boolean; P5 : Boolean; P6 : Boolean; P7 : Boolean; P8 : Boolean) return Boolean is
    (((if First_Missing (P1, P2, P3, P4, P5, P6, P7, P8) = 1 then not P1) and then (if First_Missing (P1, P2, P3, P4, P5, P6, P7, P8) = 2 then P1 and then not P2) and then (if First_Missing (P1, P2, P3, P4, P5, P6, P7, P8) = 3 then P1 and then P2 and then not P3) and then (if First_Missing (P1, P2, P3, P4, P5, P6, P7, P8) = 8 then P1 and then P2 and then P3 and then P4 and then P5 and then P6 and then P7 and then not P8)))
    with Post => Named_Fact_Is_Really_Missing'Result;

  function Examples return Boolean is
    ((Complete (True, True, True, True, True, True, True, True) and then First_Missing (True, True, True, True, True, True, True, True) = 0 and then not Complete (True, True, True, False, True, True, True, True) and then First_Missing (True, True, True, False, True, True, True, True) = 4 and then First_Missing (True, False, True, False, True, True, True, True) = 2))
    with Post => Examples'Result;

end Fact_Gate_Pkg;
